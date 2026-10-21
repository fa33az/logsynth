# logsynth

Infer a log format from real samples, emit a typed parser, and fail the build
when the format moves.

```console
$ logsynth learn examples/nginx/access.log
logsynth · examples/nginx/access.log
format   positional · 9 fields · 1 shape(s)
lines    120 read · 120 matched (100.0%)
fields
  remote_addr      ipv4     172.16.8.203
  ident            str      -  nullable
  auth_user        str      -  nullable
  time_local       ts       11/Oct/2026:15:07:34 +0700
  request          request  GET /static/app.js HTTP/1.1  [DELETE GET HEAD POST PUT]
  status           int      403
  body_bytes       int      19098
  http_referer     url      https://example.com/docs  nullable
  http_user_agent  str      sqlmap/1.8  [Go-http-client/2.0 Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126.0 Safari/537.36 curl/8.1 kube-probe/1.30 python-requests/2.32.3 sqlmap/1.8]
```

```console
$ logsynth gen examples/nginx/access.log --lang go --package nginx \
    --out examples/nginx/parser.go --test \
    --schema-out examples/nginx/access.schema.json
wrote examples/nginx/parser.go (119 lines)
wrote examples/nginx/parser_test.go (24 lines)
wrote examples/nginx/access.schema.json
inferred positional · 9 fields · 120/120 lines (100.0%)
```

```console
$ logsynth drift testdata/nginx_drift.log --schema examples/nginx/access.schema.json
baseline examples/nginx/access.log (positional, 9 fields)
current  testdata/nginx_drift.log (positional, 10 fields, 100.0% coverage)
~ type   body_bytes: int -> float
+ field  field_1 (str)
drift detected · 2 change(s)
$ echo $?
1
```

That exit code is the whole point: a log format change becomes a red build
instead of a silent 2 a.m. pager.

## Real logs, not just fixtures

Everything below ran against logs on a live Linux box, not against the repo's
own samples.

```console
$ logsynth learn /var/log/dpkg.log
logsynth · /var/log/dpkg.log
format   positional · 5 fields · 2 shape(s)
lines    2443 read · 2406 matched (98.5%)
fields
  time_1   ts    2026-10-05 07:30:11
  field_1  str   upgrade  [configure install purge remove status trigproc upgrade]
  field_2  str   libatk-adaptor:amd64
  field_3  str   2.61.90-1~bpo13+1
  field_4  str   2.62.0-2~bpo13+1
variants 1 other shape(s) · 37 line(s) not covered by the emitted parser
  w w w w w                  37x  2026-10-05 07:26:29 startup packages configure
```

The date and the clock are two whitespace tokens in the file and one `ts` field
in the schema. The generated parser then read the whole file back:

```console
$ logsynth gen /var/log/dpkg.log --lang go --package dpkg --out dpkg/parser.go
inferred positional · 5 fields · 2406/2443 lines (98.5%) · 1 other shape(s) not covered
$ go run .   # calls dpkg.Parse on every line
real dpkg.log: 2443 lines, 2406 parsed, 37 rejected (98.5%)
first: {Time1:2026-10-05 07:30:11 Field1:upgrade Field2:libatk-adaptor:amd64 Field3:2.61.90-1~bpo13+1 Field4:2.62.0-2~bpo13+1}
last:  {Time1:2026-10-21 14:26:10 Field1:status Field2:installed Field3:ca-certificates-java:all Field4:20240118}
```

The prediction and the result agree to the line, and the 37 rejects are exactly
the startup lines that were reported as a variant up front.

| log | shape | coverage | notes |
| --- | --- | --- | --- |
| `/var/log/dpkg.log` | positional | 2406/2443 (98.5%) | date + clock folded into one `ts` |
| `examples/dpkg/dpkg.log` | positional | 40/40 (100%) | real 40-line excerpt, committed |
| `/var/log/alternatives.log` | positional | 36/49 (73.5%) | folded `ts` with a trailing colon |
| `journalctl -n 300` | positional | 106/300 (35.3%) | `Oct 21 18:04:35` folded; free-text messages vary per line |
| `/var/log/Xorg.0.log` | positional | 60/587 (10.2%) | prose log, positional inference is the wrong tool |

`examples/dpkg/` holds a 40-line excerpt of that real log, a generated parser,
and a test that parses all 40 lines plus rejects the 4-column startup variant.

## Three commands, no server

| command | input | output |
| --- | --- | --- |
| `learn` | log file | field names, types, coverage, variants, schema JSON |
| `gen` | log file | Go or Python parser, optional test, optional schema |
| `drift` | log file + baseline schema | diff, exit 1 on change |

Nothing leaves the machine. No API key, no model, no network call. Same input
bytes produce byte-identical output, which is why the generated files can be
committed and diffed in review.

## Install

```console
go install github.com/fa33az/logsynth/cmd/logsynth@latest
```

From a checkout:

```console
go build -o logsynth ./cmd/logsynth
```

Requires Go 1.21+. The generated Python parser requires Python 3.9+ and no
third-party packages.

## Formats it recognises

The mode is detected from the data, never from a flag:

| mode | example | detected when |
| --- | --- | --- |
| `positional` | nginx, apache, syslog | whitespace-delimited tokens, stable field order |
| `kv` | logrus text, klog, journald | every line is `key=value` pairs (quotes may contain spaces) |
| `json` | JSONL | every line is a JSON object; nested objects flatten to `a.b` |

Field types inferred today: `ipv4`, `ipv6`, `ts` (12 layouts, including CLF,
RFC 3339 and epoch-free variants), `int`, `float`, `uuid`, `hex`, `url`,
`path`, `bool`, `request`, `enum`, `str`. `-` and empty values mark a field
`nullable` without breaking its type.

For positional logs, quoted (`"..."`) and bracketed (`[...]`) segments stay
whole, so a request line with spaces never splits into three fields.

A timestamp written with a space — `2026-10-05 07:26:29` in dpkg and apt,
`Oct 21 13:20:00` in syslog, `2026-10-05 07:31:41:` in update-alternatives — is
folded back into a single `ts` field instead of being emitted as a date column
plus a clock column. The schema records it as `"span": 2` with the layout that
matched, and the pattern matches it as one capture.

## What the generated Go looks like

```go
// Code generated by logsynth from examples/nginx/access.log. DO NOT EDIT.

package nginx

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Line is one parsed record. Numeric fields that are nullable in the
// log are pointers so an absent value stays distinguishable from zero.
type Line struct {
	RemoteAddr    string `json:"remote_addr"`
	Ident         string `json:"ident"`
	AuthUser      string `json:"auth_user"`
	TimeLocal     string `json:"time_local"`
	Request       string `json:"request"`
	Status        int64  `json:"status"`
	BodyBytes     int64  `json:"body_bytes"`
	HTTPReferer   string `json:"http_referer"`
	HTTPUserAgent string `json:"http_user_agent"`
}

var Pattern = regexp.MustCompile(`^(?P<remote_addr>\d{1,3}(?:\.\d{1,3}){3})\s+...`)

func Parse(line string) (Line, error) { /* named captures -> typed fields */ }
func ParseAll(r io.Reader) ([]Line, error) { /* bufio.Scanner, 1 MiB lines */ }
```

`examples/nginx/` ships the generated parser plus a test that reads the whole
sample log, so `go test ./...` executes generated code against real data on
every commit. `examples/app/parser.py` is the same deal for Python:

```python
@dataclass
class Line:
    time: str
    level: str
    msg: str
    service: str
    status: Optional[int]
    duration_ms: Optional[float]
    trace_id: str

def parse(line: str) -> Optional[Line]: ...
def parse_all(path: str) -> list[Line]: ...
```

## Drift in CI

Commit the schema next to the parser, then check new logs against it:

```yaml
- name: log format drift
  run: |
    go install github.com/fa33az/logsynth/cmd/logsynth@latest
    logsynth learn /var/log/app/access.log --out schema/access.json --limit 5000
    logsynth drift /var/log/app/access.log --schema schema/access.json
```

The command prints a diff and exits 1 when a field is added, removed, retyped
or reordered, or when the whole mode changes (for example kv turning into
json). Nothing else exits 1, so it drops into any pipeline unmodified.

Enum churn is reported as a note, not drift:

```console
$ logsynth drift /var/log/dpkg.log --schema examples/dpkg/dpkg.schema.json
baseline examples/dpkg/dpkg.log (positional, 5 fields)
current  /var/log/dpkg.log (positional, 5 fields, 98.5% coverage)
~ enum   field_1: +configure +install +purge +remove +trigproc
~ enum   field_2: -at-spi2-common:all -at-spi2-core:amd64 ...
no structural drift · 4 enum note(s)
$ echo $?
0
```

Package names and user agents change with traffic; the parser does not reject a
line because an enum gained a value. Treating that as drift would make the gate
cry wolf on the first busy day, so it is printed and then ignored. A dropped
column still fails:

```console
$ logsynth drift testdata/dpkg_dropped.log --schema examples/dpkg/dpkg.schema.json
- field  field_3
- field  field_4
drift detected · 2 change(s)
$ echo $?
1
```

`drift` also accepts two schema files, which is handy when the baseline lives
on another branch:

```console
logsynth drift main.schema.json pr.schema.json
```

## How it works

1. Tokenise each line into structural tokens, keeping quotes and brackets
   intact, and cluster lines by shape (`w w w b q w w q q`).
2. Fold whitespace-separated timestamps back together, then pick the dominant
   shape, infer a type per column, and merge votes: a `float` beside `int`
   widens to `float`, anything else widens to `str` — the only type that can
   never reject a later line.
3. Name the fields. A recognised combined-log prefix gets canonical nginx
   names, which is why a widened `body_bytes` reports one type change instead
   of nine renames.
4. Build an anchored regexp from the per-field patterns and hand it to the
   emitter.
5. Emit code through `go/format`, so a generator bug fails at emit time rather
   than in your tree.

## Limits, stated plainly

- Only the dominant shape is emitted. Other shapes are reported with counts and
  examples, never silently parsed — coverage tells you exactly what is missing.
- Positional inference assumes stable field order. Logs that reorder fields per
  line fall back to a lower coverage number; nothing pretends otherwise.
- No multi-line records yet. Stack traces attached to a line become part of the
  last quoted field or turn into a separate variant.
- Enum detection caps at 12 distinct values; past that the field stays `str`
  with no enum.
- Free-text log lines (Xorg logs, journald messages) defeat positional
  inference: the message column takes a different number of tokens per line, so
  coverage lands in the 10-40% range. That number is reported, not hidden.
- No streaming throughput tuning. `ParseAll` is a single-pass scanner; measure
  it on your data instead of trusting a README number.
- Generated parsers are intentionally dependency-free, which means they carry
  their own helpers instead of importing a library.

## Repo layout

```
cmd/logsynth/          CLI: learn, gen, drift
internal/tokenize/     line -> structural tokens; key=value scanner
internal/infer/        mode detection, type inference, naming, pattern build
internal/schema/       schema JSON, diffing, version checks
internal/emit/         Go and Python emitters
examples/nginx/        generated Go parser + schema + full-log test
examples/dpkg/         generated Go parser from a real dpkg.log excerpt
examples/app/          generated Python parser
examples/events/       generated JSONL parser + full-log test
testdata/              fixtures, including the drift demo logs
```

## License

MIT — see `LICENSE`.
