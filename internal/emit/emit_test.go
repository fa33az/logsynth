package emit

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/fa33az/logsynth/internal/infer"
	"github.com/fa33az/logsynth/internal/schema"
)

var (
	nginxSample = []string{
		`93.184.216.34 - - [10/Oct/2026:13:55:36 +0700] "GET /api/v1/items/42 HTTP/1.1" 200 512 "-" "curl/8.1"`,
		`10.0.4.19 - - [10/Oct/2026:13:55:37 +0700] "POST /api/v1/orders HTTP/1.1" 201 184 "-" "python-requests/2.32.3"`,
	}
	kvSample = []string{
		`time="2026-10-11T13:48:09+07:00" level=info msg="request handled" status=200 duration_ms=12.5`,
		`time="2026-10-11T13:48:10+07:00" level=error msg="payment failed" status=500 duration_ms=93.25`,
	}
	jsonSample = []string{
		`{"ts":"2026-10-11T13:48:09.123Z","level":"info","http":{"status":200},"latency_ms":12.5}`,
		`{"ts":"2026-10-11T13:48:10.123Z","level":"warn","http":{"status":500},"latency_ms":8.25}`,
	}
)

func mustSchema(t *testing.T, lines []string) *schema.Schema {
	t.Helper()
	s, err := infer.FromLines("sample.log", lines, infer.Options{})
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	return s
}

func mustGo(t *testing.T, s *schema.Schema, opts GoOptions) (string, string) {
	t.Helper()
	src, test, err := Go(s, opts)
	if err != nil {
		t.Fatalf("Go: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "parser.go", src, 0); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	return src, test
}

func requireContains(t *testing.T, src string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(src, want) {
			t.Errorf("generated source missing %q", want)
		}
	}
}

func TestGoPositional(t *testing.T) {
	s := mustSchema(t, nginxSample)
	src, test := mustGo(t, s, GoOptions{Package: "access", Sample: nginxSample[:1]})
	requireContains(t, src,
		"package access",
		"type Line struct",
		"var Pattern = regexp.MustCompile",
		"func Parse(line string) (Line, error)",
		"func ParseAll(r io.Reader) ([]Line, error)",
		"func fieldMap(",
		"RemoteAddr",
		`json:"remote_addr"`,
	)
	if test == "" {
		t.Fatal("expected a generated test file")
	}
	requireContains(t, test, "func TestParseSample", "const sampleLines")
	if _, err := parser.ParseFile(token.NewFileSet(), "parser_test.go", test, 0); err != nil {
		t.Fatalf("generated test does not parse: %v", err)
	}
}

func TestGoKV(t *testing.T) {
	s := mustSchema(t, kvSample)
	src, _ := mustGo(t, s, GoOptions{Package: "app"})
	requireContains(t, src, "package app", "func splitPairs(", `case "level":`, "func parseInt(")
}

func TestGoJSON(t *testing.T) {
	s := mustSchema(t, jsonSample)
	src, _ := mustGo(t, s, GoOptions{Package: "events"})
	requireContains(t, src, `"encoding/json"`, "func flatten(", "func asInt64(", `flat["http.status"]`)
}

func TestGoIsDeterministic(t *testing.T) {
	s := mustSchema(t, nginxSample)
	first, _, err := Go(s, GoOptions{Package: "access"})
	if err != nil {
		t.Fatalf("Go: %v", err)
	}
	second, _, err := Go(s, GoOptions{Package: "access"})
	if err != nil {
		t.Fatalf("Go: %v", err)
	}
	if first != second {
		t.Fatal("generated output is not deterministic")
	}
}

func TestPythonPositional(t *testing.T) {
	s := mustSchema(t, nginxSample)
	src, err := Python(s)
	if err != nil {
		t.Fatalf("Python: %v", err)
	}
	requireContains(t, src,
		"@dataclass",
		"class Line:",
		"PATTERN = re.compile",
		"def parse(line: str) -> Optional[Line]:",
		"def parse_all(path: str) -> list[Line]:",
	)
}

func TestPythonKVAndJSON(t *testing.T) {
	kv, err := Python(mustSchema(t, kvSample))
	if err != nil {
		t.Fatalf("Python(kv): %v", err)
	}
	requireContains(t, kv, "_PAIR = re.compile", "def _pairs(")

	js, err := Python(mustSchema(t, jsonSample))
	if err != nil {
		t.Fatalf("Python(json): %v", err)
	}
	requireContains(t, js, "import json", "def _flatten(", `flat.get("http.status")`)
}
