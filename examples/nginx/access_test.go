package nginx

import (
	"os"
	"testing"
)

// TestParseFullLog runs the generated parser over the whole sample log. This is
// the proof that "learn" and "gen" agree: every line the schema claimed to
// cover parses into a record.
func TestParseFullLog(t *testing.T) {
	fh, err := os.Open("access.log")
	if err != nil {
		t.Fatalf("open access.log: %v", err)
	}
	defer fh.Close()

	records, err := ParseAll(fh)
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	if len(records) != 120 {
		t.Fatalf("got %d records, want 120", len(records))
	}

	for i, record := range records {
		if record.RemoteAddr == "" {
			t.Fatalf("line %d: empty remote_addr", i+1)
		}
		if record.Status < 100 || record.Status > 599 {
			t.Fatalf("line %d: status out of range: %d", i+1, record.Status)
		}
		if record.TimeLocal == "" {
			t.Fatalf("line %d: empty time_local", i+1)
		}
	}

	// access.log line 8 is pinned by the generator script.
	pinned := records[7]
	if pinned.Status != 200 || pinned.BodyBytes != 512 || pinned.HTTPUserAgent != "curl/8.1" {
		t.Errorf("pinned record = %+v", pinned)
	}
	if pinned.HTTPReferer != "-" {
		t.Errorf("pinned referer = %q, want -", pinned.HTTPReferer)
	}
}
