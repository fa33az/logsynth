package main

import (
	"errors"
	"testing"

	"github.com/fa33az/logsynth/internal/schema"
)

func driftFixture() *schema.Schema {
	return &schema.Schema{
		Version: schema.CurrentVersion,
		Mode:    schema.ModePositional,
		Source:  "dpkg.log",
		Lines:   40,
		Matched: 40,
		Fields: []schema.Field{
			{Name: "time_1", Type: "ts", Index: 0, Span: 2, Layout: "2006-01-02 15:04:05"},
			{Name: "field_1", Type: "str", Index: 2, Enum: []string{"install", "upgrade"}},
			{Name: "field_2", Type: "str", Index: 3},
		},
	}
}

// Real logs churn their enum columns every day: new package names, new user
// agents. That must not fail a CI gate, because the parser still reads every
// line. This is the behaviour the /var/log/dpkg.log run demanded.
func TestReportDriftIgnoresEnumChurn(t *testing.T) {
	baseline := driftFixture()
	current := driftFixture()
	current.Source = "/var/log/dpkg.log"
	current.Fields[1].Enum = []string{"install", "purge", "status", "upgrade"}
	if err := reportDrift(baseline, current); err != nil {
		t.Fatalf("enum churn reported as drift: %v", err)
	}
}

func TestReportDriftFlagsStructuralChange(t *testing.T) {
	baseline := driftFixture()
	current := driftFixture()
	current.Fields[2].Type = "int"
	if err := reportDrift(baseline, current); !errors.Is(err, errDrift) {
		t.Fatalf("err = %v, want errDrift", err)
	}
}

func TestReportDriftFlagsDroppedTimestampColumn(t *testing.T) {
	baseline := driftFixture()
	current := driftFixture()
	current.Fields = current.Fields[1:]
	if err := reportDrift(baseline, current); !errors.Is(err, errDrift) {
		t.Fatalf("err = %v, want errDrift", err)
	}
}
