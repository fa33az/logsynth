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

func TestLineGluesProcessTag(t *testing.T) {
	toks, err := Line(`Oct 21 18:18:38 parrot orca[50013]: scan roots=51 present=4`)
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	want := []string{"Oct", "21", "18:18:38", "parrot", "orca[50013]:", "scan", "roots=51", "present=4"}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %#v", len(toks), len(want), toks)
	}
	for i := range want {
		if toks[i].Value != want[i] {
			t.Errorf("token %d = %q, want %q", i, toks[i].Value, want[i])
		}
		if toks[i].Kind != Word {
			t.Errorf("token %d kind = %v, want Word", i, toks[i].Kind)
		}
	}
	if got, wantShape := Shape(toks), "w w w w w w w w"; got != wantShape {
		t.Errorf("Shape = %q, want %q", got, wantShape)
	}
}

func TestLineGluesRepeatedBrackets(t *testing.T) {
	toks, err := Line(`a[b][c]: rest`)
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if got, want := toks[0].Value, "a[b][c]:"; got != want {
		t.Errorf("token 0 = %q, want %q", got, want)
	}
	if got := len(toks); got != 2 {
		t.Errorf("got %d tokens, want 2: %#v", got, toks)
	}
}

func TestLineDoesNotGlueSpacedBracket(t *testing.T) {
	toks, err := Line(`foo [bar] baz`)
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if len(toks) != 3 {
		t.Fatalf("got %d tokens, want 3: %#v", len(toks), toks)
	}
	if toks[0].Value != "foo" || toks[0].Kind != Word {
		t.Errorf("token 0 = %q/%v, want foo/Word", toks[0].Value, toks[0].Kind)
	}
	if toks[1].Value != "bar" || toks[1].Kind != Bracketed {
		t.Errorf("token 1 = %q/%v, want bar/Bracketed", toks[1].Value, toks[1].Kind)
	}
}

func TestLineUnclosedBracketStaysWord(t *testing.T) {
	toks, err := Line(`a[b rest`)
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if got := len(toks); got != 4 {
		t.Fatalf("got %d tokens, want 4: %#v", got, toks)
	}
	if toks[0].Value != "a" || toks[1].Value != "[" || toks[2].Value != "b" {
		t.Errorf("tokens = %#v", toks)
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
