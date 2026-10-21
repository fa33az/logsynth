package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fa33az/logsynth/internal/emit"
	"github.com/fa33az/logsynth/internal/infer"
	"github.com/fa33az/logsynth/internal/schema"
)

func runGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	lang := fs.String("lang", "go", "output language: go | python")
	pkg := fs.String("package", "", `Go package name (default: derived from --out, else "logs")`)
	outPath := fs.String("out", "", "write the parser here instead of stdout")
	withTest := fs.Bool("test", false, "also write <name>_test.go (Go only)")
	schemaOut := fs.String("schema-out", "", "also write the inferred schema JSON here")
	limit := fs.Int("limit", 0, "sample at most N lines (0 = all)")
	if err := fs.Parse(normalizeArgs(args, map[string]bool{
		"lang": true, "package": true, "out": true, "schema-out": true, "limit": true,
	})); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("gen: missing log file")
	}

	path := fs.Arg(0)
	lines, err := infer.ReadLinesLimit(path, *limit)
	if err != nil {
		return err
	}
	s, err := infer.FromLines(path, lines, infer.Options{MaxSampleLines: *limit})
	if err != nil {
		return err
	}

	var src, test string
	switch strings.ToLower(*lang) {
	case "go", "golang":
		packageName := *pkg
		if packageName == "" {
			packageName = defaultPackage(*outPath)
		}
		if err := validPackage(packageName); err != nil {
			return err
		}
		src, test, err = emit.Go(s, emit.GoOptions{
			Package: packageName,
			Sample:  pickSamples(lines, 3),
		})
		if err != nil {
			return err
		}
	case "python", "py":
		if *withTest {
			return errors.New("gen: --test only applies to --lang go")
		}
		src, err = emit.Python(s)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("gen: unknown language %q (want go or python)", *lang)
	}

	if *schemaOut != "" {
		if err := s.WriteFile(*schemaOut); err != nil {
			return err
		}
	}

	if *outPath == "" {
		fmt.Print(src)
		fmt.Fprintln(os.Stderr, summarize(s, "stdout"))
		return nil
	}
	if dir := filepath.Dir(*outPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(*outPath, []byte(src), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d lines)\n", *outPath, countLines(src))
	if test != "" {
		testPath := strings.TrimSuffix(*outPath, filepath.Ext(*outPath)) + "_test.go"
		if err := os.WriteFile(testPath, []byte(test), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d lines)\n", testPath, countLines(test))
	}
	if *schemaOut != "" {
		fmt.Fprintf(os.Stderr, "wrote %s\n", *schemaOut)
	}
	fmt.Fprintln(os.Stderr, summarize(s, ""))
	return nil
}

func summarize(s *schema.Schema, target string) string {
	msg := fmt.Sprintf("inferred %s · %d fields · %d/%d lines (%.1f%%)",
		s.Mode, len(s.Fields), s.Matched, s.Lines, 100*s.Coverage())
	if len(s.Variants) > 0 {
		msg += fmt.Sprintf(" · %d other shape(s) not covered", len(s.Variants))
	}
	if target != "" {
		msg += " · " + target
	}
	return msg
}

func defaultPackage(outPath string) string {
	if outPath == "" {
		return "logs"
	}
	base := filepath.Base(outPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSuffix(base, "_parser")
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			return r
		}
		return '_'
	}, base)
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return "logs"
	}
	return cleaned
}

func validPackage(name string) error {
	if name == "" {
		return errors.New("gen: empty package name")
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			continue
		case r >= '0' && r <= '9' && i > 0:
			continue
		}
		return fmt.Errorf("gen: %q is not a valid Go package name", name)
	}
	return nil
}

func countLines(s string) int {
	return strings.Count(s, "\n")
}
