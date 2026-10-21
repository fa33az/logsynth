package infer

import (
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fa33az/logsynth/internal/schema"
	"github.com/fa33az/logsynth/internal/tokenize"
)

var (
	uuidRe    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	floatRe   = regexp.MustCompile(`^-?\d+\.\d+(?:[eE][-+]?\d+)?$`)
	requestRe = regexp.MustCompile(`^[A-Z]{3,9} \S+ HTTP/\d(?:\.\d)?$`)

	// tsLayouts covers the timestamp shapes that actually show up in logs.
	// The first successful parse wins and the layouts do not overlap in
	// practice, so the order is only cosmetic.
	tsLayouts = []string{
		"02/Jan/2006:15:04:05 -0700",
		"02/Jan/2006:15:04:05",
		"2/Jan/2006:15:04:05 -0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02",
		"Jan _2 15:04:05",
		"Jan _2 2006 15:04:05",
	}

	// mergedTSLayouts are the layouts whose values contain whitespace, so one
	// logical timestamp arrives as two or three tokens. Ordered longest first:
	// "Jan _2 2006 15:04:05" must win before "Jan _2 15:04:05" can see it.
	mergedTSLayouts = []string{
		"Jan _2 2006 15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"02/Jan/2006 15:04:05",
		"Jan _2 15:04:05",
	}
)

// detectType classifies one raw value. "dash" marks a nullable placeholder
// ("-" or empty) and is excluded from type voting.
func detectType(value string, kind tokenize.Kind) string {
	if value == "" || value == "-" {
		return "dash"
	}
	if kind == tokenize.Bracketed {
		if isTimestamp(value) {
			return "ts"
		}
		if isURL(value) {
			return "url"
		}
		if isPath(value) {
			return "path"
		}
		return "str"
	}
	if isTimestamp(value) {
		return "ts"
	}
	if net.ParseIP(value) != nil {
		if strings.Contains(value, ":") {
			return "ipv6"
		}
		return "ipv4"
	}
	if uuidRe.MatchString(value) {
		return "uuid"
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return "int"
	}
	if floatRe.MatchString(value) {
		return "float"
	}
	if isHexID(value) {
		return "hex"
	}
	switch {
	case value == "true" || value == "false":
		return "bool"
	case strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X"):
		return "hex"
	case isURL(value):
		return "url"
	case isPath(value):
		return "path"
	case requestRe.MatchString(value):
		return "request"
	}
	return "str"
}

func isURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

// isHexID recognises long hex identifiers (trace ids, request ids). A candidate
// must contain at least one a-f letter so plain integers — including epoch
// nanoseconds — keep their int type.
func isHexID(value string) bool {
	if len(value) < 16 {
		return false
	}
	hasLetter := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
			hasLetter = true
		default:
			return false
		}
	}
	return hasLetter
}

func isPath(value string) bool {
	return strings.HasPrefix(value, "/") && !strings.ContainsAny(value, " \t")
}

func isTimestamp(value string) bool {
	if len(value) < 8 {
		return false
	}
	for _, layout := range tsLayouts {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

// mergedTimestampLayout reports which layout parses a timestamp that spans
// whitespace. A single trailing colon is tolerated because update-alternatives
// and a few syslog writers put one between the time and the message.
func mergedTimestampLayout(value string) (string, bool) {
	value = strings.TrimSuffix(value, ":")
	for _, layout := range mergedTSLayouts {
		if _, err := time.Parse(layout, value); err == nil {
			return layout, true
		}
	}
	return "", false
}

// classify turns one column of values into a Field.
func classify(index int, values []string, kinds []tokenize.Kind, maxEnum int) schema.Field {
	field := schema.Field{Index: index}
	counts := make(map[string]int, 4)
	samples := make(map[string]string, 4)
	kindCounts := make(map[tokenize.Kind]int, 3)
	dash := 0

	for i, value := range values {
		kind := tokenize.Word
		if i < len(kinds) {
			kind = kinds[i]
		}
		kindCounts[kind]++
		detected := detectType(value, kind)
		if detected == "dash" {
			dash++
			continue
		}
		counts[detected]++
		if _, ok := samples[detected]; !ok {
			samples[detected] = value
		}
	}

	field.Nullable = dash > 0
	field.Kind = dominantKind(kindCounts).Name()
	field.Type = dominantType(counts)
	field.Sample = samples[field.Type]
	if field.Sample == "" && field.Nullable {
		field.Sample = "-"
	}

	switch field.Type {
	case "request":
		field.Enum = methods(values)
	case "str", "bool":
		field.Enum = enumValues(values, maxEnum)
	}
	return field
}

func dominantKind(counts map[tokenize.Kind]int) tokenize.Kind {
	best, bestCount := tokenize.Word, -1
	for _, kind := range []tokenize.Kind{tokenize.Word, tokenize.Quoted, tokenize.Bracketed} {
		if counts[kind] > bestCount {
			best, bestCount = kind, counts[kind]
		}
	}
	return best
}

// dominantType resolves a type vote. int+float collapses to float because every
// int parses as a float; any other mix collapses to str, the only type that can
// never reject a later line.
func dominantType(counts map[string]int) string {
	if len(counts) == 0 {
		return "str"
	}
	types := make([]string, 0, len(counts))
	for t := range counts {
		types = append(types, t)
	}
	sort.Strings(types)
	if len(types) == 1 {
		return types[0]
	}
	if len(types) == 2 && types[0] == "float" && types[1] == "int" {
		return "float"
	}
	return "str"
}

func enumValues(values []string, maxEnum int) []string {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || value == "-" {
			continue
		}
		seen[value] = true
	}
	if len(seen) == 0 || len(seen) > maxEnum {
		return nil
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func methods(values []string) []string {
	seen := make(map[string]bool, 8)
	for _, value := range values {
		if i := strings.IndexByte(value, ' '); i > 0 {
			seen[value[:i]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for method := range seen {
		out = append(out, method)
	}
	sort.Strings(out)
	return out
}
