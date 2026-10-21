// Package schema defines the intermediate representation shared by the
// inference stage and the code emitters.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// CurrentVersion is the on-disk schema format version. Bump it whenever the
// JSON shape changes incompatibly; Load rejects anything else.
const CurrentVersion = 1

// Mode is the structural family of a log format.
type Mode string

const (
	// ModePositional covers delimiter-separated records where field order is
	// stable across lines (nginx, apache, syslog, most "space separated" logs).
	ModePositional Mode = "positional"
	// ModeKV covers key=value records (logrus text, journald, klog).
	ModeKV Mode = "kv"
	// ModeJSON covers one JSON object per line (JSONL).
	ModeJSON Mode = "json"
)

// Field is one inferred column.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Index is the position of the field's first token on the line.
	Index int    `json:"index"`
	Key   string `json:"key,omitempty"`
	Kind  string `json:"kind,omitempty"`
	// Span is how many whitespace tokens the value occupies. It is 2 or 3 for
	// timestamps written with a space ("2026-10-05 07:26:29", "Oct 21 13:20:00"),
	// which are one value in every log that uses them.
	Span     int      `json:"span,omitempty"`
	Layout   string   `json:"layout,omitempty"`
	Nullable bool     `json:"nullable,omitempty"`
	Enum     []string `json:"enum,omitempty"`
	Sample   string   `json:"sample,omitempty"`
}

// Variant records a shape that was seen but is not covered by the emitted
// parser. Coverage counts only the dominant shape.
type Variant struct {
	Shape   string `json:"shape"`
	Count   int    `json:"count"`
	Example string `json:"example,omitempty"`
}

// Schema is the full inference result. It is stable JSON so it can be
// committed next to the code it generates.
type Schema struct {
	Version  int       `json:"version"`
	Mode     Mode      `json:"mode"`
	Source   string    `json:"source"`
	Lines    int       `json:"lines"`
	Matched  int       `json:"matched"`
	Pattern  string    `json:"pattern,omitempty"`
	Fields   []Field   `json:"fields"`
	Variants []Variant `json:"variants,omitempty"`
}

// Coverage is the fraction of input lines the emitted parser handles.
func (s *Schema) Coverage() float64 {
	if s.Lines == 0 {
		return 0
	}
	return float64(s.Matched) / float64(s.Lines)
}

// Marshal renders the schema as indented JSON with the version stamped. HTML
// escaping is off so regexps stay readable in review diffs.
func (s *Schema) Marshal() ([]byte, error) {
	s.Version = CurrentVersion
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// WriteFile renders and writes the schema to path.
func (s *Schema) WriteFile(path string) error {
	blob, err := s.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(blob, '\n'), 0o644)
}

// Load reads a schema file and validates its version.
func Load(path string) (*Schema, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Schema
	if err := json.Unmarshal(blob, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Version != CurrentVersion {
		return nil, fmt.Errorf("%s: schema version %d, want %d", path, s.Version, CurrentVersion)
	}
	return &s, nil
}

// Change is a single structural difference between two schemas.
type Change struct {
	Kind   string
	Field  string
	Detail string
	// Note marks an informational change. Enum sets move with traffic and never
	// make the generated parser reject a line, so they are reported but do not
	// count as drift. Otherwise every CI run on real logs fails on package names
	// and user agents alone.
	Note bool
}

func (c Change) String() string {
	switch c.Kind {
	case "added":
		return "+ field  " + c.Field + " (" + c.Detail + ")"
	case "removed":
		return "- field  " + c.Field
	case "type":
		return "~ type   " + c.Field + ": " + c.Detail
	case "order":
		return "~ order  " + c.Field + ": " + c.Detail
	case "enum":
		return "~ enum   " + c.Field + ": " + c.Detail
	case "mode":
		return "~ mode   " + c.Field + ": " + c.Detail
	}
	return c.Kind + " " + c.Field
}

// Diff compares two schemas. A field is matched by name, so a renamed field
// shows up as one removal plus one addition. That is intentional: the emitted
// struct member changes too.
func Diff(old, new *Schema) []Change {
	oldByName := make(map[string]Field, len(old.Fields))
	for _, f := range old.Fields {
		oldByName[f.Name] = f
	}
	newByName := make(map[string]Field, len(new.Fields))
	for _, f := range new.Fields {
		newByName[f.Name] = f
	}

	var changes []Change
	if old.Mode != new.Mode {
		changes = append(changes, Change{
			Kind:   "mode",
			Field:  "format",
			Detail: string(old.Mode) + " -> " + string(new.Mode),
		})
	}
	for _, f := range new.Fields {
		prev, ok := oldByName[f.Name]
		if !ok {
			changes = append(changes, Change{Kind: "added", Field: f.Name, Detail: f.Type})
			continue
		}
		if prev.Type != f.Type {
			changes = append(changes, Change{Kind: "type", Field: f.Name, Detail: prev.Type + " -> " + f.Type})
		}
		if old.Mode == ModePositional && new.Mode == ModePositional && prev.Index != f.Index {
			changes = append(changes, Change{Kind: "order", Field: f.Name, Detail: fmt.Sprintf("%d -> %d", prev.Index, f.Index)})
		}
		if !sameStrings(prev.Enum, f.Enum) {
			changes = append(changes, Change{Kind: "enum", Field: f.Name, Detail: describeEnum(prev.Enum, f.Enum), Note: true})
		}
	}
	for _, f := range old.Fields {
		if _, ok := newByName[f.Name]; !ok {
			changes = append(changes, Change{Kind: "removed", Field: f.Name})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Field < changes[j].Field })
	return changes
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			// Enum values are sorted, but tolerate an unsorted rewrite.
			return false
		}
	}
	return true
}

func describeEnum(old, new []string) string {
	oldSet := make(map[string]bool, len(old))
	for _, v := range old {
		oldSet[v] = true
	}
	newSet := make(map[string]bool, len(new))
	for _, v := range new {
		newSet[v] = true
	}
	var parts []string
	for _, v := range new {
		if !oldSet[v] {
			parts = append(parts, "+"+quote(v))
		}
	}
	for _, v := range old {
		if !newSet[v] {
			parts = append(parts, "-"+quote(v))
		}
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func quote(s string) string {
	if s == "" {
		return `""`
	}
	needs := false
	for _, r := range s {
		if r == ' ' || r == '"' || r == '=' {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	return `"` + s + `"`
}
