package infer

import (
	"fmt"
	"strings"

	"github.com/fa33az/logsynth/internal/schema"
)

// nameFields gives every field a stable, readable name. Known combined-log
// shapes get their canonical nginx/apache names; anything else gets a
// type-derived name with an ordinal, never a bare "field1".
func nameFields(fields []schema.Field) []string {
	if names := canonicalNames(fields); names != nil {
		return names
	}
	return genericNames(fields)
}

// genericNames assigns <type>_<ordinal> names, which stay unique by
// construction.
func genericNames(fields []schema.Field) []string {
	names := make([]string, len(fields))
	used := make(map[string]int, len(fields))
	for i, field := range fields {
		base := baseName(field)
		used[base]++
		names[i] = fmt.Sprintf("%s_%d", base, used[base])
	}
	return names
}

// canonicalNames recognises the combined-log prefix rather than an exact field
// count. Logs gain columns all the time; matching on the prefix keeps existing
// names stable so `drift` reports the real change instead of a wall of renames.
func canonicalNames(fields []schema.Field) []string {
	if len(fields) < 7 {
		return nil
	}
	types := make([]string, len(fields))
	for i := range fields {
		types[i] = fields[i].Type
	}
	// Anchor on structure, not on every type: a widened numeric column must not
	// rename the whole layout, or drift output becomes noise.
	commonLog := types[0] == "ipv4" && fields[3].Kind == "bracketed" &&
		types[3] == "ts" && types[4] == "request"
	if !commonLog {
		return nil
	}
	names := []string{
		"remote_addr", "ident", "auth_user", "time_local", "request",
		"status", "body_bytes",
	}
	if len(fields) >= 9 {
		names = append(names, "http_referer", "http_user_agent")
	}
	if len(fields) > len(names) {
		names = append(names, genericNames(fields[len(names):])...)
	}
	return names
}

func baseName(field schema.Field) string {
	switch field.Type {
	case "ipv4":
		return "ip"
	case "ipv6":
		return "ip6"
	case "ts":
		return "time"
	case "int":
		return "int"
	case "float":
		return "float"
	case "uuid":
		return "id"
	case "url":
		return "url"
	case "path":
		return "path"
	case "hex":
		return "hex"
	case "bool":
		return "flag"
	case "request":
		return "request"
	}
	return "field"
}

// BuildPattern renders the anchored regexp that reproduces every field of the
// dominant shape. Quoted and bracketed fields keep their delimiters so the
// pattern cannot silently merge two adjacent fields.
func BuildPattern(fields []schema.Field) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fragment(field))
	}
	return "^" + strings.Join(parts, `\s+`) + "$"
}

func fragment(field schema.Field) string {
	if field.Type == "ts" && field.Span > 1 {
		return "(?P<" + field.Name + ">" + tsSpanFragment(field.Layout) + ")"
	}
	inner := `\S+`
	switch field.Kind {
	case "quoted":
		inner = `[^"]*`
	case "bracketed":
		inner = `[^\]]*`
	default:
		switch field.Type {
		case "int":
			inner = `-?\d+`
		case "float":
			inner = `-?\d+(?:\.\d+)?`
		case "uuid":
			inner = `[0-9a-fA-F]{8}-(?:[0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}`
		case "ipv4":
			inner = `\d{1,3}(?:\.\d{1,3}){3}`
		}
	}
	if field.Nullable && field.Kind == "word" {
		inner = "(?:" + inner + `|-)`
	}
	group := "(?P<" + field.Name + ">" + inner + ")"
	switch field.Kind {
	case "quoted":
		return `"` + group + `"`
	case "bracketed":
		return `\[` + group + `\]`
	}
	return group
}

// tsSpanFragment renders the regexp for a timestamp that spans whitespace. The
// optional trailing colon covers writers like update-alternatives, which put a
// colon between the clock and the message.
func tsSpanFragment(layout string) string {
	switch layout {
	case "Jan _2 2006 15:04:05":
		return `[A-Z][a-z]{2}\s+\d{1,2}\s+\d{4}\s+\d{2}:\d{2}:\d{2}:?`
	case "2006-01-02 15:04:05.000":
		return `\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+:?`
	case "2006-01-02 15:04:05":
		return `\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}:?`
	case "2006/01/02 15:04:05":
		return `\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2}:?`
	case "02/Jan/2006 15:04:05":
		return `\d{1,2}/[A-Z][a-z]{2}/\d{4}\s+\d{2}:\d{2}:\d{2}:?`
	case "Jan _2 15:04:05":
		return `[A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}:?`
	}
	// Unknown layout: match the token count without guessing the shape.
	return `\S+(?:\s+\S+)+`
}
