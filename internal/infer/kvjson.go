package infer

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/fa33az/logsynth/internal/schema"
	"github.com/fa33az/logsynth/internal/tokenize"
)

func allJSON(lines []string) bool {
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "{") {
			return false
		}
		var probe map[string]any
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			return false
		}
	}
	return len(lines) > 0
}

func allKV(lines []string) bool {
	for _, line := range lines {
		if _, ok := tokenize.KV(line); !ok {
			return false
		}
	}
	return len(lines) > 0
}

func inferKV(s *schema.Schema, lines []string, opts Options) error {
	order := make([]string, 0, 8)
	seen := make(map[string]bool, 8)
	values := make(map[string][]string, 8)

	for _, line := range lines {
		pairs, ok := tokenize.KV(line)
		if !ok {
			continue
		}
		s.Matched++
		for _, pair := range pairs {
			if !seen[pair.Key] {
				seen[pair.Key] = true
				order = append(order, pair.Key)
			}
			values[pair.Key] = append(values[pair.Key], pair.Value)
		}
	}
	if len(order) == 0 {
		return fmt.Errorf("%s: no key=value line", s.Source)
	}

	s.Mode = schema.ModeKV
	s.Fields = make([]schema.Field, 0, len(order))
	for i, key := range order {
		vals := values[key]
		kinds := make([]tokenize.Kind, len(vals))
		for j := range kinds {
			kinds[j] = tokenize.Word
		}
		field := classify(i, vals, kinds, opts.maxEnum())
		field.Key = key
		field.Name = sanitizeKey(key)
		s.Fields = append(s.Fields, field)
	}
	return nil
}

func inferJSON(s *schema.Schema, lines []string, opts Options) error {
	seen := make(map[string]bool, 16)
	order := make([]string, 0, 16)
	values := make(map[string][]string, 16)

	for _, line := range lines {
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		s.Matched++
		flat := make(map[string]any, len(raw))
		flattenJSON("", raw, flat, 3)

		keys := make([]string, 0, len(flat))
		for key := range flat {
			keys = append(keys, key)
		}
		// JSON objects carry no field order, so sorting gives a stable schema
		// across runs regardless of map iteration order.
		sort.Strings(keys)
		for _, key := range keys {
			if !seen[key] {
				seen[key] = true
				order = append(order, key)
			}
			values[key] = append(values[key], jsonValueString(flat[key]))
		}
	}
	if len(order) == 0 {
		return fmt.Errorf("%s: no JSON object line", s.Source)
	}
	sort.Strings(order)

	s.Mode = schema.ModeJSON
	s.Fields = make([]schema.Field, 0, len(order))
	for i, key := range order {
		vals := values[key]
		kinds := make([]tokenize.Kind, len(vals))
		for j := range kinds {
			kinds[j] = tokenize.Word
		}
		field := classify(i, vals, kinds, opts.maxEnum())
		field.Key = key
		field.Name = sanitizeKey(key)
		s.Fields = append(s.Fields, field)
	}
	return nil
}

// flattenJSON flattens nested objects into dotted keys, up to depth levels.
// Arrays stay whole so they keep their original JSON sample.
func flattenJSON(prefix string, in map[string]any, out map[string]any, depth int) {
	for key, value := range in {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok && depth > 1 {
			flattenJSON(full, nested, out, depth-1)
			continue
		}
		out[full] = value
	}
}

func jsonValueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return "-"
	case string:
		return typed
	case float64:
		if typed == math.Trunc(typed) && math.Abs(typed) < 1e15 {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		blob, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(blob)
	}
}

// sanitizeKey turns an arbitrary log key into an identifier-safe field name.
func sanitizeKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "field"
	}
	return out
}
