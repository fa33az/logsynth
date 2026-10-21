package tokenize

import (
	"errors"
	"testing"
)

func TestLinePositional(t *testing.T) {
	line := `93.184.216.34 - - [10/Oct/2026:13:55:36 +0700] "GET /api/v1/items/42 HTTP/1.1" 200 512 "-" "curl/8.1"`
	toks, err := Line(line)
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	want := []struct {
		value string
		kind  Kind
	}{
		{"93.184.216.34", Word},
		{"-", Word},
		{"-", Word},
		{"10/Oct/2026:13:55:36 +0700", Bracketed},
		{"GET /api/v1/items/42 HTTP/1.1", Quoted},
		{"200", Word},
		{"512", Word},
		{"-", Quoted},
		{"curl/8.1", Quoted},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %#v", len(toks), len(want), toks)
	}
	for i := range want {
		if toks[i].Value != want[i].value || toks[i].Kind != want[i].kind {
			t.Errorf("token %d = %q/%v, want %q/%v", i, toks[i].Value, toks[i].Kind, want[i].value, want[i].kind)
		}
	}
	if got, want := Shape(toks), "w w w b q w w q q"; got != want {
		t.Errorf("Shape = %q, want %q", got, want)
	}
}

func TestLineEscapedQuote(t *testing.T) {
	toks, err := Line(`a "he said \"hi\"" b`)
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if len(toks) != 3 {
		t.Fatalf("got %d tokens: %#v", len(toks), toks)
	}
	if toks[1].Value != `he said "hi"` {
		t.Errorf("quoted value = %q", toks[1].Value)
	}
}

func TestLineErrors(t *testing.T) {
	if _, err := Line("   \t "); !errors.Is(err, ErrEmpty) {
		t.Errorf("blank line err = %v, want ErrEmpty", err)
	}
	if _, err := Line(`a "unterminated`); err == nil {
		t.Error("unterminated quote: want error")
	}
}

func TestKV(t *testing.T) {
	line := `time="2026-10-11T13:48:09+07:00" level=info msg="request handled" status=200 duration_ms=12.5`
	pairs, ok := KV(line)
	if !ok {
		t.Fatal("KV returned false")
	}
	want := map[string]string{
		"time":        "2026-10-11T13:48:09+07:00",
		"level":       "info",
		"msg":         "request handled",
		"status":      "200",
		"duration_ms": "12.5",
	}
	if len(pairs) != len(want) {
		t.Fatalf("got %d pairs, want %d: %#v", len(pairs), len(want), pairs)
	}
	for _, pair := range pairs {
		if want[pair.Key] != pair.Value {
			t.Errorf("%s = %q, want %q", pair.Key, pair.Value, want[pair.Key])
		}
	}
}

func TestKVRejectsPlainText(t *testing.T) {
	for _, line := range []string{
		"just some words here",
		"a=1",
		"key=value trailing",
	} {
		if _, ok := KV(line); ok {
			t.Errorf("KV(%q) = true, want false", line)
		}
	}
}
