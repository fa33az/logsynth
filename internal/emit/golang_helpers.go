package emit

// Generated helper blocks. They are raw string literals, so Go escapes inside
// them (like \t) survive into the emitted source exactly as written.

const fieldMapHelper = `func fieldMap(match []string) map[string]string {
	out := make(map[string]string, len(match))
	for i, name := range Pattern.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		out[name] = match[i]
	}
	return out
}

`

const scalarHelpers = `func parseInt(s string) (int64, bool) {
	if s == "" || s == "-" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseFloat(s string) (float64, bool) {
	if s == "" || s == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseBool(s string) (bool, bool) {
	switch s {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

`

const kvHelper = `type pair struct {
	key   string
	value string
}

// splitPairs reads key=value pairs, honouring double-quoted values that may
// contain spaces.
func splitPairs(line string) []pair {
	out := make([]pair, 0, 8)
	i, n := 0, len(line)
	for i < n {
		for i < n && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= n {
			break
		}
		start := i
		for i < n && line[i] != '=' && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		if i >= n || line[i] != '=' {
			break
		}
		key := line[start:i]
		i++
		var value string
		if i < n && line[i] == '"' {
			var sb strings.Builder
			i++
			for i < n {
				if line[i] == '\\' && i+1 < n {
					sb.WriteByte(line[i+1])
					i += 2
					continue
				}
				if line[i] == '"' {
					i++
					break
				}
				sb.WriteByte(line[i])
				i++
			}
			value = sb.String()
		} else {
			start = i
			for i < n && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			value = line[start:i]
		}
		out = append(out, pair{key: key, value: value})
	}
	return out
}

`

const jsonHelpers = `func flatten(prefix string, in map[string]any, out map[string]any, depth int) {
	for key, value := range in {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok && depth > 1 {
			flatten(full, nested, out, depth-1)
			continue
		}
		out[full] = value
	}
}

func asString(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func asInt64(v any) (int64, bool) {
	switch typed := v.(type) {
	case float64:
		return int64(typed), true
	case string:
		n, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func asFloat(v any) (float64, bool) {
	switch typed := v.(type) {
	case float64:
		return typed, true
	case string:
		f, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func asBool(v any) (bool, bool) {
	switch typed := v.(type) {
	case bool:
		return typed, true
	case string:
		if typed == "true" || typed == "1" {
			return true, true
		}
		if typed == "false" || typed == "0" {
			return false, true
		}
	}
	return false, false
}

`
