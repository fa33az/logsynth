// Command logsynth infers log formats and emits parsers that reproduce them.
//
//	logsynth learn <log>                     show the inferred schema
//	logsynth gen   <log> --lang go --out f   write a parser
//	logsynth drift <log> --schema old.json   fail when the format moved
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fa33az/logsynth/internal/infer"
	"github.com/fa33az/logsynth/internal/schema"
)

// errDrift signals "format changed" and maps to exit code 1 without an extra
// message, because the diff has already been printed.
var errDrift = errors.New("log format drifted")

const usage = `logsynth - infer log formats, generate parsers, catch drift

usage:
  logsynth learn <log> [--json] [--out schema.json] [--limit N]
  logsynth gen   <log> --lang go|python [--out file] [--package name]
                       [--test] [--schema-out schema.json] [--limit N]
  logsynth drift <log> --schema baseline.json
  logsynth drift <old.json> <new.json>
  logsynth version

exit codes:
  0  success / no drift
  1  drift detected, or an error
  2  bad usage

formats detected automatically: positional (nginx, apache, syslog),
key=value (logrus, klog), JSON lines.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "learn":
		err = runLearn(os.Args[2:])
	case "gen":
		err = runGen(os.Args[2:])
	case "drift":
		err = runDrift(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("logsynth v0.1.0")
		return
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "logsynth: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err == nil {
		return
	}
	if errors.Is(err, errDrift) {
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "logsynth: "+err.Error())
	os.Exit(1)
}

func runLearn(args []string) error {
	fs := flag.NewFlagSet("learn", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	asJSON := fs.Bool("json", false, "print the inferred schema as JSON")
	outPath := fs.String("out", "", "also write the schema JSON to this path")
	limit := fs.Int("limit", 0, "sample at most N lines (0 = all)")
	if err := fs.Parse(normalizeArgs(args, map[string]bool{"out": true, "limit": true})); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("learn: missing log file")
	}

	path := fs.Arg(0)
	s, err := infer.FromFile(path, infer.Options{MaxSampleLines: *limit})
	if err != nil {
		return err
	}
	if *outPath != "" {
		if err := s.WriteFile(*outPath); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *outPath)
	}
	if *asJSON {
		blob, err := s.Marshal()
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", blob)
		return nil
	}
	printLearn(s)
	return nil
}

func printLearn(s *schema.Schema) {
	nameWidth, typeWidth := len("field"), len("type")
	for _, field := range s.Fields {
		if len(field.Name) > nameWidth {
			nameWidth = len(field.Name)
		}
		if len(field.Type) > typeWidth {
			typeWidth = len(field.Type)
		}
	}

	fmt.Printf("logsynth · %s\n", s.Source)
	fmt.Printf("format   %s · %d fields · %d shape(s)\n", s.Mode, len(s.Fields), len(s.Variants)+1)
	fmt.Printf("lines    %d read · %d matched (%.1f%%)\n", s.Lines, s.Matched, 100*s.Coverage())
	fmt.Println("fields")
	for _, field := range s.Fields {
		line := fmt.Sprintf("  %-*s  %-*s  %s", nameWidth, field.Name, typeWidth, field.Type, clip(field.Sample, 46))
		if len(field.Enum) > 0 {
			line += "  [" + strings.Join(field.Enum, " ") + "]"
		}
		if field.Nullable {
			line += "  nullable"
		}
		fmt.Println(strings.TrimRight(line, " "))
	}
	if len(s.Variants) > 0 {
		uncovered := s.Lines - s.Matched
		fmt.Printf("variants %d other shape(s) · %d line(s) not covered by the emitted parser\n", len(s.Variants), uncovered)
		for _, variant := range s.Variants {
			fmt.Printf("  %-24s %4dx  %s\n", variant.Shape, variant.Count, clip(variant.Example, 70))
		}
	}
}

func clip(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

// normalizeArgs moves positional arguments to the end, so "gen access.log
// --lang go" parses exactly like "gen --lang go access.log". The stdlib flag
// package stops at the first positional argument, and humans write the file
// name first.
func normalizeArgs(args []string, valueFlags map[string]bool) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				continue
			}
			if valueFlags[name] && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

func pickSamples(lines []string, limit int) []string {
	out := make([]string, 0, limit)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) == limit {
			break
		}
	}
	return out
}
