package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func sampleSchema() *Schema {
	return &Schema{
		Version: CurrentVersion,
		Mode:    ModePositional,
		Source:  "access.log",
		Lines:   10,
		Matched: 10,
		Fields: []Field{
			{Name: "remote_addr", Type: "ipv4", Index: 0, Kind: "word"},
			{Name: "time_local", Type: "ts", Index: 1, Kind: "bracketed"},
			{Name: "status", Type: "int", Index: 2, Kind: "word"},
			{Name: "level", Type: "str", Index: 3, Kind: "word", Enum: []string{"error", "info"}},
		},
	}
}

func TestDiffIdentical(t *testing.T) {
	changes := Diff(sampleSchema(), sampleSchema())
	if len(changes) != 0 {
		t.Fatalf("changes = %#v, want none", changes)
	}
}

func TestDiffDetectsChanges(t *testing.T) {
	baseline := sampleSchema()
	current := sampleSchema()
	current.Fields[2].Type = "float"                           // status widened
	current.Fields[3].Enum = []string{"error", "info", "warn"} // new level
	current.Fields = append(current.Fields, Field{Name: "bytes", Type: "int", Index: 4})
	current.Fields[0].Index = 5 // reordered

	got := make(map[string]Change, len(Diff(baseline, current)))
	for _, change := range Diff(baseline, current) {
		got[change.Kind+":"+change.Field] = change
	}
	if got["type:status"].Detail != "int -> float" {
		t.Errorf("type change = %#v", got["type:status"])
	}
	if _, ok := got["added:bytes"]; !ok {
		t.Error("missing added:bytes")
	}
	if got["enum:level"].Detail != "+warn" {
		t.Errorf("enum detail = %q, want +warn", got["enum:level"].Detail)
	}
	if !got["enum:level"].Note {
		t.Error("enum change must be a note, not drift")
	}
	if got["type:status"].Note {
		t.Error("type change must count as drift")
	}
	if _, ok := got["order:remote_addr"]; !ok {
		t.Error("missing order:remote_addr")
	}
}

func TestDiffDetectsRemovalAndMode(t *testing.T) {
	baseline := sampleSchema()
	current := sampleSchema()
	current.Fields = current.Fields[:3]
	current.Mode = ModeJSON

	seen := map[string]bool{}
	for _, change := range Diff(baseline, current) {
		seen[change.Kind] = true
		if change.Kind == "removed" && change.Field != "level" {
			t.Errorf("removed field = %q, want level", change.Field)
		}
	}
	if !seen["mode"] || !seen["removed"] {
		t.Errorf("kinds = %v, want mode and removed", seen)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	original := sampleSchema()
	if err := original.WriteFile(path); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Mode != original.Mode || len(loaded.Fields) != len(original.Fields) {
		t.Fatalf("round trip mismatch: %#v", loaded)
	}
	if loaded.Fields[3].Enum[1] != "info" {
		t.Errorf("enum lost: %#v", loaded.Fields[3])
	}
}

func TestLoadRejectsBadVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"mode":"kv","fields":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("want version error")
	}
}

func TestCoverage(t *testing.T) {
	s := sampleSchema()
	if s.Coverage() != 1 {
		t.Errorf("coverage = %v", s.Coverage())
	}
	s.Matched = 5
	if s.Coverage() != 0.5 {
		t.Errorf("coverage = %v", s.Coverage())
	}
	s.Lines = 0
	if s.Coverage() != 0 {
		t.Errorf("coverage = %v, want 0", s.Coverage())
	}
}
