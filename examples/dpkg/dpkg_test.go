package dpkg

import (
	"os"
	"strings"
	"testing"
)

// Every line of the committed excerpt is a real dpkg.log line, so this is a
// parse-everything check, not a spot check.
func TestParseEveryLineOfRealLog(t *testing.T) {
	raw, err := os.ReadFile("dpkg.log")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 40 {
		t.Fatalf("fixture changed: %d lines, want 40", len(lines))
	}
	for i, line := range lines {
		rec, err := Parse(line)
		if err != nil {
			t.Fatalf("line %d: %v", i+1, err)
		}
		if rec.Time1 == "" || rec.Field1 == "" {
			t.Fatalf("line %d parsed empty: %+v", i+1, rec)
		}
	}
}

// The date and the clock are two whitespace tokens in the file and one field in
// the parser. This is the regression guard for that fold.
func TestTimestampFold(t *testing.T) {
	rec, err := Parse("2026-10-05 07:30:11 upgrade libatk-adaptor:amd64 2.61.90-1~bpo13+1 2.62.0-2~bpo13+1")
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-10-05 07:30:11"
	if rec.Time1 != want {
		t.Fatalf("Time1 = %q, want %q", rec.Time1, want)
	}
	if rec.Field1 != "upgrade" || rec.Field2 != "libatk-adaptor:amd64" {
		t.Fatalf("fields landed in the wrong columns: %+v", rec)
	}
}

// A startup line has no version column; the emitted pattern must reject it
// instead of guessing, because coverage is reported as 40/40 not 40/41.
func TestVariantLineIsRejected(t *testing.T) {
	if _, err := Parse("2026-10-05 07:26:29 startup packages configure"); err == nil {
		t.Fatal("expected the 4-column startup line to be rejected")
	}
}
