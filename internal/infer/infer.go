// Package infer turns raw log lines into a schema. It decides the structural
// mode (positional, key=value, JSON) from the data itself, never from a flag,
// then infers field types from the values it sees.
package infer

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fa33az/logsynth/internal/schema"
	"github.com/fa33az/logsynth/internal/tokenize"
)

const (
	defaultMaxEnum = 12
	maxVariants    = 5
	// maxGroupedLines caps the per-variant token matrix. 5000 lines is enough
	// for stable type inference and keeps memory at a few MB on 1 GB logs.
	maxGroupedLines = 5000
	// maxLineBytes is the scanner ceiling. Real access logs with fat user
	// agents and stack traces stay well under this.
	maxLineBytes = 1 << 20
)

// Options tunes inference.
type Options struct {
	// MaxEnum caps how many distinct values still count as an enum.
	// Zero means defaultMaxEnum.
	MaxEnum int
	// MaxSampleLines caps how many lines are read. Zero means all of them.
	MaxSampleLines int
}

func (o Options) maxEnum() int {
	if o.MaxEnum > 0 {
		return o.MaxEnum
	}
	return defaultMaxEnum
}

// ReadLines reads a log file into memory, skipping blank lines.
func ReadLines(path string) ([]string, error) {
	return ReadLinesLimit(path, 0)
}

// ReadLinesLimit reads at most max non-empty lines from path. The scanner stops
// as soon as the sample is full, so `learn --limit` on a multi-gigabyte log
// reads a bounded prefix instead of loading the whole file and slicing it.
// max <= 0 reads every line.
func ReadLinesLimit(path string, max int) ([]string, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	var lines []string
	for sc.Scan() {
		if max > 0 && len(lines) >= max {
			break
		}
		raw := strings.TrimRight(sc.Text(), " \t\r")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		lines = append(lines, raw)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return lines, nil
}

// FromFile reads path and infers its schema.
func FromFile(path string, opts Options) (*schema.Schema, error) {
	lines, err := ReadLinesLimit(path, opts.MaxSampleLines)
	if err != nil {
		return nil, err
	}
	return FromLines(path, lines, opts)
}

// FromLines infers a schema from raw lines.
func FromLines(source string, raw []string, opts Options) (*schema.Schema, error) {
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s: no non-empty lines", source)
	}
	if opts.MaxSampleLines > 0 && len(lines) > opts.MaxSampleLines {
		lines = lines[:opts.MaxSampleLines]
	}

	s := &schema.Schema{Version: schema.CurrentVersion, Source: source, Lines: len(lines)}
	switch {
	case allJSON(lines):
		if err := inferJSON(s, lines, opts); err != nil {
			return nil, err
		}
	case allKV(lines):
		if err := inferKV(s, lines, opts); err != nil {
			return nil, err
		}
	default:
		if err := inferPositional(s, lines, opts); err != nil {
			return nil, err
		}
	}
	if s.Matched == 0 {
		return nil, fmt.Errorf("%s: no line matched a known format", source)
	}
	return s, nil
}

type variantGroup struct {
	count   int
	example string
	tokens  [][]tokenize.Token
}

func inferPositional(s *schema.Schema, lines []string, opts Options) error {
	groups := make(map[string]*variantGroup, 4)
	var order []string
	for _, line := range lines {
		toks, err := tokenize.Line(line)
		if err != nil {
			continue
		}
		key := tokenize.Shape(toks)
		group, ok := groups[key]
		if !ok {
			group = &variantGroup{}
			groups[key] = group
			order = append(order, key)
		}
		group.count++
		if group.example == "" {
			group.example = line
		}
		if len(group.tokens) < maxGroupedLines {
			group.tokens = append(group.tokens, toks)
		}
	}
	if len(groups) == 0 {
		return fmt.Errorf("%s: no line could be tokenised", s.Source)
	}
	sort.SliceStable(order, func(i, j int) bool {
		return groups[order[i]].count > groups[order[j]].count
	})

	dominant := groups[order[0]]
	s.Mode = schema.ModePositional
	s.Matched = dominant.count
	for _, key := range order[1:] {
		if len(s.Variants) >= maxVariants {
			break
		}
		group := groups[key]
		s.Variants = append(s.Variants, schema.Variant{
			Shape:   key,
			Count:   group.count,
			Example: truncate(group.example, 160),
		})
	}

	fields := inferFields(dominant.tokens, opts)
	s.Fields = fields
	s.Pattern = BuildPattern(fields)
	return nil
}

func inferFields(rows [][]tokenize.Token, opts Options) []schema.Field {
	cols := buildColumns(rows)
	fields := make([]schema.Field, 0, len(cols))
	for _, col := range cols {
		if len(col.cells) == 0 {
			continue
		}
		field := classify(col.start, col.cells, col.kinds, opts.maxEnum())
		if col.span > 1 {
			// The merge fired only after every row parsed as one timestamp, so
			// the type is structural, not a vote: a trailing colon or a layout
			// the single-token table does not know must not demote it to str.
			field.Type = "ts"
			field.Kind = tokenize.Word.Name()
			field.Span = col.span
			field.Layout = col.layout
			field.Enum = nil
			field.Sample = col.cells[0]
		}
		fields = append(fields, field)
	}
	names := nameFields(fields)
	for i := range fields {
		fields[i].Name = names[i]
	}
	return fields
}

// column is one field's worth of values, before it becomes a schema.Field.
// span is 1 for an ordinary whitespace token and 2-4 when a timestamp written
// with spaces was folded back together.
type column struct {
	start  int
	cells  []string
	kinds  []tokenize.Kind
	span   int
	layout string
}

// buildColumns slices the token matrix into columns. A timestamp split by
// whitespace ("2026-10-05 07:26:29" in dpkg and apt, "Oct 21 13:20:00" in
// syslog) is folded into a single column, because emitting a date field and a
// clock field would make every caller reassemble them.
func buildColumns(rows [][]tokenize.Token) []column {
	width := 0
	for _, toks := range rows {
		if len(toks) > width {
			width = len(toks)
		}
	}
	cols := make([]column, 0, width)
	for start := 0; start < width; {
		if col, ok := mergedColumn(rows, start); ok {
			cols = append(cols, col)
			start += col.span
			continue
		}
		col := column{start: start, span: 1}
		for _, toks := range rows {
			if start >= len(toks) {
				continue
			}
			col.cells = append(col.cells, toks[start].Value)
			col.kinds = append(col.kinds, toks[start].Kind)
		}
		cols = append(cols, col)
		start++
	}
	return cols
}

// mergedColumn tries the whitespace-spanning timestamp layouts at start,
// longest span first. It only fires when every row agrees, so a column that is
// a date for some lines and a full timestamp for others stays untouched.
func mergedColumn(rows [][]tokenize.Token, start int) (column, bool) {
	for _, span := range []int{4, 3, 2} {
		layout, ok := sharedMergedLayout(rows, start, span)
		if !ok {
			continue
		}
		col := column{start: start, span: span, layout: layout}
		for _, toks := range rows {
			cells := make([]string, 0, span)
			for i := start; i < start+span; i++ {
				cells = append(cells, toks[i].Value)
			}
			col.cells = append(col.cells, strings.Join(cells, " "))
			col.kinds = append(col.kinds, tokenize.Word)
		}
		return col, true
	}
	return column{}, false
}

// sharedMergedLayout returns the layout that parses every row's span, or false
// when any row is short, quoted, bracketed, or does not parse.
func sharedMergedLayout(rows [][]tokenize.Token, start, span int) (string, bool) {
	layout := ""
	for _, toks := range rows {
		if start+span > len(toks) {
			return "", false
		}
		cells := make([]string, 0, span)
		for i := start; i < start+span; i++ {
			if toks[i].Kind != tokenize.Word {
				return "", false
			}
			cells = append(cells, toks[i].Value)
		}
		got, ok := mergedTimestampLayout(strings.Join(cells, " "))
		if !ok {
			return "", false
		}
		if layout == "" {
			layout = got
		}
	}
	return layout, layout != ""
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit-1] + "…"
}
