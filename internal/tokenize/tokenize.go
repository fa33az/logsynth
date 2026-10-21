// Package tokenize splits a single log line into the structural pieces the
// inference stage reasons about. It is deliberately allocation-light and
// dependency-free so generated parsers can copy the same rules.
package tokenize

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEmpty is returned for lines that contain no tokens.
var ErrEmpty = errors.New("tokenize: empty line")

// Kind is the structural class of a token. It decides how the token is
// re-emitted into a regexp: quoted tokens are wrapped in `"`, bracketed tokens
// in `[]`, words stay bare.
type Kind uint8

const (
	// Word is a bare whitespace-delimited token.
	Word Kind = iota
	// Quoted is a "double quoted" token. Quotes are stripped from Value.
	Quoted
	// Bracketed is a [square bracketed] token. Brackets are stripped.
	Bracketed
)

// Code is the single-byte shape code used to cluster lines.
func (k Kind) Code() byte {
	switch k {
	case Quoted:
		return 'q'
	case Bracketed:
		return 'b'
	}
	return 'w'
}

// Name is the JSON spelling stored in schema.Field.Kind.
func (k Kind) Name() string {
	switch k {
	case Quoted:
		return "quoted"
	case Bracketed:
		return "bracketed"
	}
	return "word"
}

// Token is one structural piece of a line.
type Token struct {
	Value string
	Kind  Kind
}

// Line splits a log line into positional tokens. Quoted and bracketed segments
// stay whole even when they contain spaces; everything else splits on runs of
// space, tab or CR.
func Line(line string) ([]Token, error) {
	var toks []Token
	i, n := 0, len(line)
	for i < n {
		switch c := line[i]; {
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '"':
			value, next, err := readQuoted(line, i)
			if err != nil {
				return nil, fmt.Errorf("tokenize: %w", err)
			}
			toks = append(toks, Token{Value: value, Kind: Quoted})
			i = next
		case c == '[':
			if end := strings.IndexByte(line[i:], ']'); end > 0 {
				toks = append(toks, Token{Value: line[i+1 : i+end], Kind: Bracketed})
				i += end + 1
				continue
			}
			toks = append(toks, Token{Value: "[", Kind: Word})
			i++
		default:
			start := i
			for i < n {
				ch := line[i]
				if ch == ' ' || ch == '\t' || ch == '\r' || ch == '"' || ch == '[' {
					break
				}
				i++
			}
			toks = append(toks, Token{Value: line[start:i], Kind: Word})
		}
	}
	if len(toks) == 0 {
		return nil, ErrEmpty
	}
	return toks, nil
}

// Shape renders the structural signature of a token slice, for example
// "w w w b q w w q q". Lines with the same shape are one format variant.
func Shape(toks []Token) string {
	var b strings.Builder
	b.Grow(len(toks) * 2)
	for i, t := range toks {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(t.Kind.Code())
	}
	return b.String()
}

// Pair is one key=value entry.
type Pair struct {
	Key   string
	Value string
}

// KV parses a key=value line such as `time="..." level=info msg="hi there"`.
// It returns false when the line is not a well-formed pair list (fewer than two
// pairs, or a fragment without '=').
func KV(line string) ([]Pair, bool) {
	pairs := make([]Pair, 0, 8)
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
			return nil, false
		}
		key := line[start:i]
		if key == "" {
			return nil, false
		}
		i++ // consume '='
		var value string
		if i < n && line[i] == '"' {
			v, next, err := readQuoted(line, i)
			if err != nil {
				return nil, false
			}
			value, i = v, next
		} else {
			start = i
			for i < n && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			value = line[start:i]
		}
		pairs = append(pairs, Pair{Key: key, Value: value})
	}
	if len(pairs) < 2 {
		return nil, false
	}
	return pairs, true
}

func readQuoted(s string, start int) (string, int, error) {
	var b strings.Builder
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) {
				return "", 0, errors.New("unterminated escape")
			}
			i++
			b.WriteByte(s[i])
		case '"':
			return b.String(), i + 1, nil
		default:
			b.WriteByte(s[i])
		}
	}
	return "", 0, errors.New("unterminated quote")
}
