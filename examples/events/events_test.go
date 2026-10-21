package events

import (
	"os"
	"testing"
)

// TestParseFullLog drives the generated JSON parser over every sample event.
func TestParseFullLog(t *testing.T) {
	fh, err := os.Open("events.jsonl")
	if err != nil {
		t.Fatalf("open events.jsonl: %v", err)
	}
	defer fh.Close()

	records, err := ParseAll(fh)
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	if len(records) != 60 {
		t.Fatalf("got %d records, want 60", len(records))
	}
	for i, record := range records {
		if record.HTTPPath == "" || record.HTTPMethod == "" {
			t.Fatalf("line %d: missing nested http fields: %+v", i+1, record)
		}
		if record.HTTPStatus < 100 || record.HTTPStatus > 599 {
			t.Fatalf("line %d: status out of range: %d", i+1, record.HTTPStatus)
		}
		if record.TraceID == "" || record.Service != "gateway" {
			t.Fatalf("line %d: bad identity fields: %+v", i+1, record)
		}
	}
}
