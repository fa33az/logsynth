package infer

import (
	"regexp"
	"strings"
	"testing"

	"github.com/fa33az/logsynth/internal/schema"
)

var nginxLines = []string{
	`93.184.216.34 - - [10/Oct/2026:13:55:36 +0700] "GET /api/v1/items/42 HTTP/1.1" 200 512 "-" "curl/8.1"`,
	`10.0.4.19 - - [10/Oct/2026:13:55:37 +0700] "POST /api/v1/orders HTTP/1.1" 201 184 "-" "python-requests/2.32.3"`,
	`8.8.4.4 - alice [10/Oct/2026:13:55:38 +0700] "PUT /health HTTP/1.1" 304 0 "https://example.com/docs" "Mozilla/5.0 (X11; Linux x86_64) Chrome/126.0"`,
}

func TestPositionalNginx(t *testing.T) {
	s, err := FromLines("access.log", nginxLines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if s.Mode != schema.ModePositional {
		t.Fatalf("mode = %q", s.Mode)
	}
	if s.Coverage() != 1 {
		t.Errorf("coverage = %v, want 1", s.Coverage())
	}

	wantNames := []string{
		"remote_addr", "ident", "auth_user", "time_local", "request",
		"status", "body_bytes", "http_referer", "http_user_agent",
	}
	wantTypes := []string{
		"ipv4", "str", "str", "ts", "request",
		"int", "int", "url", "str",
	}
	if len(s.Fields) != len(wantNames) {
		t.Fatalf("got %d fields, want %d: %#v", len(s.Fields), len(wantNames), s.Fields)
	}
	for i, field := range s.Fields {
		if field.Name != wantNames[i] {
			t.Errorf("field %d name = %q, want %q", i, field.Name, wantNames[i])
		}
		if field.Type != wantTypes[i] {
			t.Errorf("field %d (%s) type = %q, want %q", i, field.Name, field.Type, wantTypes[i])
		}
	}
	if got := s.Fields[1].Nullable; !got {
		t.Error("ident should be nullable (dash values)")
	}
	if got := s.Pattern; got == "" {
		t.Fatal("empty pattern")
	}
	pattern, err := regexp.Compile(s.Pattern)
	if err != nil {
		t.Fatalf("pattern does not compile: %v", err)
	}
	for _, line := range nginxLines {
		if !pattern.MatchString(line) {
			t.Errorf("pattern does not match %q", line)
		}
	}
	if methods := s.Fields[4].Enum; len(methods) != 3 {
		t.Errorf("request enum = %v, want 3 methods", methods)
	}
}

func TestPositionalVariants(t *testing.T) {
	lines := append([]string{
		`1.2.3.4 - - [10/Oct/2026:13:55:36 +0700] "GET /a HTTP/1.1" 200 10`,
	}, nginxLines...)
	s, err := FromLines("mixed.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if s.Matched != 3 {
		t.Errorf("matched = %d, want 3", s.Matched)
	}
	if len(s.Variants) != 1 {
		t.Fatalf("variants = %d, want 1: %#v", len(s.Variants), s.Variants)
	}
	if s.Variants[0].Count != 1 || s.Variants[0].Shape != "w w w b q w w" {
		t.Errorf("variant = %#v", s.Variants[0])
	}
	if s.Coverage() != 0.75 {
		t.Errorf("coverage = %v, want 0.75", s.Coverage())
	}
}

func TestKVMode(t *testing.T) {
	lines := []string{
		`time="2026-10-11T13:48:09+07:00" level=info msg="request handled" status=200 duration_ms=12.5`,
		`time="2026-10-11T13:48:10+07:00" level=error msg="payment failed" status=500 duration_ms=93.25`,
	}
	s, err := FromLines("app.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if s.Mode != schema.ModeKV {
		t.Fatalf("mode = %q", s.Mode)
	}
	want := map[string]string{
		"time": "ts", "level": "str", "msg": "str",
		"status": "int", "duration_ms": "float",
	}
	if len(s.Fields) != len(want) {
		t.Fatalf("got %d fields: %#v", len(s.Fields), s.Fields)
	}
	for _, field := range s.Fields {
		if want[field.Name] != field.Type {
			t.Errorf("%s type = %q, want %q", field.Name, field.Type, want[field.Name])
		}
	}
	if enum := s.Fields[1].Enum; len(enum) != 2 || enum[0] != "error" {
		t.Errorf("level enum = %v", enum)
	}
}

func TestJSONMode(t *testing.T) {
	lines := []string{
		`{"ts":"2026-10-11T13:48:09.123Z","level":"info","http":{"status":200,"path":"/health"},"latency_ms":12.5,"trace":"a3f1c9d2e4b5a6978877665544332211"}`,
		`{"ts":"2026-10-11T13:48:10.123Z","level":"warn","http":{"status":500,"path":"/login"},"latency_ms":8.25,"trace":"b3f1c9d2e4b5a6978877665544332211"}`,
	}
	s, err := FromLines("events.jsonl", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if s.Mode != schema.ModeJSON {
		t.Fatalf("mode = %q", s.Mode)
	}
	got := make(map[string]string, len(s.Fields))
	for _, field := range s.Fields {
		got[field.Name] = field.Type
	}
	want := map[string]string{
		"http_path": "path", "http_status": "int", "latency_ms": "float",
		"level": "str", "trace": "hex", "ts": "ts",
	}
	for name, wantType := range want {
		if got[name] != wantType {
			t.Errorf("%s type = %q, want %q", name, got[name], wantType)
		}
	}
}

func TestMixedTypesFallBackToString(t *testing.T) {
	lines := []string{"seq=1 value=200", "seq=2 value=1.5", "seq=3 value=pending"}
	s, err := FromLines("mixed.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if s.Fields[1].Type != "str" {
		t.Errorf("value type = %q, want str", s.Fields[1].Type)
	}
}

func TestNullableNumericStaysTyped(t *testing.T) {
	lines := []string{
		`a=1 status=200`,
		`a=2 status=404`,
		`a=3 status=-`,
	}
	s, err := FromLines("null.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	status := s.Fields[1]
	if status.Type != "int" || !status.Nullable {
		t.Errorf("status = %+v, want nullable int", status)
	}
}

func TestCanonicalNamesSurviveTypeWidening(t *testing.T) {
	// Two widened lines and one classic line: the 10-token shape wins, and the
	// canonical names must survive the body_bytes int->float change. This is the
	// regression that keeps `drift` output readable.
	lines := []string{
		`1.2.3.4 - - [10/Oct/2026:13:55:36 +0700] "GET /a HTTP/1.1" 200 10.5 "-" "curl/8.1" "request_time=0.004"`,
		`1.2.3.5 - - [10/Oct/2026:13:55:37 +0700] "GET /b HTTP/1.1" 200 11.5 "-" "curl/8.1" "request_time=0.005"`,
		`1.2.3.6 - - [10/Oct/2026:13:55:38 +0700] "GET /c HTTP/1.1" 404 12 "-" "curl/8.1"`,
	}
	s, err := FromLines("drift.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	wantNames := []string{
		"remote_addr", "ident", "auth_user", "time_local", "request",
		"status", "body_bytes", "http_referer", "http_user_agent", "field_1",
	}
	if len(s.Fields) != len(wantNames) {
		t.Fatalf("got %d fields, want %d", len(s.Fields), len(wantNames))
	}
	for i, field := range s.Fields {
		if field.Name != wantNames[i] {
			t.Errorf("field %d name = %q, want %q", i, field.Name, wantNames[i])
		}
	}
	if s.Fields[6].Type != "float" {
		t.Errorf("body_bytes type = %q, want float", s.Fields[6].Type)
	}
}

// A date and a clock written with a space are one value, not two columns. This
// is the dpkg/apt shape and the regression that started it.
func TestMergedTimestampAcrossTokens(t *testing.T) {
	lines := []string{
		"2026-10-05 07:30:11 upgrade libatk-adaptor:amd64 2.61.90-1~bpo13+1 2.62.0-2~bpo13+1",
		"2026-10-05 07:30:11 status half-configured libatk-adaptor:amd64 2.61.90-1~bpo13+1",
		"2026-10-05 07:30:12 configure libc-bin:amd64 2.41-7 2.41-7",
	}
	s, err := FromLines("dpkg.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if len(s.Fields) != 5 {
		t.Fatalf("got %d fields, want 5 (timestamp folded): %#v", len(s.Fields), s.Fields)
	}
	stamp := s.Fields[0]
	if stamp.Type != "ts" || stamp.Span != 2 {
		t.Fatalf("stamp = %+v, want ts with span 2", stamp)
	}
	if stamp.Sample != "2026-10-05 07:30:11" {
		t.Fatalf("sample = %q, want the joined timestamp", stamp.Sample)
	}
	pattern, err := regexp.Compile(s.Pattern)
	if err != nil {
		t.Fatalf("pattern does not compile: %v", err)
	}
	for _, line := range lines {
		m := pattern.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("pattern does not match %q", line)
		}
		want := strings.Join(strings.Fields(line)[:2], " ")
		if got := m[pattern.SubexpIndex(stamp.Name)]; got != want {
			t.Fatalf("captured %q, want %q", got, want)
		}
	}
}

// syslog writes the month and the clock with a space too, so the fold spans
// three tokens there. Longest span wins, otherwise "Oct" would start a column.
func TestMergedSyslogTimestamp(t *testing.T) {
	lines := []string{
		"Oct 21 18:39:30 parrot node[48921]: health probe failed",
		"Oct 21 18:39:42 parrot kwin[2264]: draw buffers failed",
	}
	s, err := FromLines("syslog", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if s.Fields[0].Type != "ts" || s.Fields[0].Span != 3 {
		t.Fatalf("first field = %+v, want ts with span 3", s.Fields[0])
	}
	if s.Fields[0].Sample != "Oct 21 18:39:30" {
		t.Fatalf("sample = %q", s.Fields[0].Sample)
	}
}

// update-alternatives separates the clock from the message with a colon. The
// fold must swallow it; a trailing colon is punctuation, not a column.
func TestMergedTimestampTrailingColon(t *testing.T) {
	lines := []string{
		"update-alternatives 2026-10-05 07:31:41: run with --install /usr/bin/x x /usr/bin/firefox 70",
		"update-alternatives 2026-10-05 07:31:43: run with --install /usr/bin/y y /usr/bin/chrome 70",
	}
	s, err := FromLines("alternatives.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	stamp := s.Fields[1]
	if stamp.Type != "ts" || stamp.Span != 2 {
		t.Fatalf("stamp = %+v, want ts with span 2", stamp)
	}
	if stamp.Sample != "2026-10-05 07:31:41:" {
		t.Fatalf("sample = %q", stamp.Sample)
	}
	pattern := regexp.MustCompile(s.Pattern)
	for _, line := range lines {
		if !pattern.MatchString(line) {
			t.Fatalf("pattern does not match %q", line)
		}
	}
}

// A date column with no clock next to it must stay its own field: folding is a
// whitespace-adjacency rule, not a guess about intent.
func TestDateWithoutClockDoesNotFold(t *testing.T) {
	lines := []string{
		"2026-10-05 daily report generated by cron",
		"2026-10-06 daily report generated by cron",
	}
	s, err := FromLines("cron.log", lines, Options{})
	if err != nil {
		t.Fatalf("FromLines: %v", err)
	}
	if s.Fields[0].Span != 0 {
		t.Fatalf("date field folded: %+v", s.Fields[0])
	}
	if s.Fields[0].Type != "ts" {
		t.Fatalf("date type = %q, want ts", s.Fields[0].Type)
	}
}

func TestEmptyInput(t *testing.T) {
	if _, err := FromLines("empty.log", []string{"", "   "}, Options{}); err == nil {
		t.Fatal("want error for empty input")
	}
}
