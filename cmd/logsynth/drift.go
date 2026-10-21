package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/fa33az/logsynth/internal/infer"
	"github.com/fa33az/logsynth/internal/schema"
)

func runDrift(args []string) error {
	fs := flag.NewFlagSet("drift", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	schemaPath := fs.String("schema", "", "baseline schema JSON to compare against")
	limit := fs.Int("limit", 0, "sample at most N lines (0 = all)")
	if err := fs.Parse(normalizeArgs(args, map[string]bool{"schema": true, "limit": true})); err != nil {
		return err
	}

	var baseline, current *schema.Schema
	var err error
	switch {
	case *schemaPath != "":
		if fs.NArg() < 1 {
			return errors.New("drift: missing log file")
		}
		if baseline, err = schema.Load(*schemaPath); err != nil {
			return err
		}
		path := fs.Arg(0)
		if current, err = infer.FromFile(path, infer.Options{MaxSampleLines: *limit}); err != nil {
			return err
		}
	default:
		if fs.NArg() < 2 {
			return errors.New("drift: want <log> --schema <baseline.json>, or <old.json> <new.json>")
		}
		if baseline, err = schema.Load(fs.Arg(0)); err != nil {
			return err
		}
		if current, err = schema.Load(fs.Arg(1)); err != nil {
			return err
		}
	}
	return reportDrift(baseline, current)
}

// reportDrift prints the difference and returns errDrift when there is one, so
// the process exits 1 inside a pipeline.
func reportDrift(baseline, current *schema.Schema) error {
	fmt.Printf("baseline %s (%s, %d fields)\n", baseline.Source, baseline.Mode, len(baseline.Fields))
	fmt.Printf("current  %s (%s, %d fields, %.1f%% coverage)\n",
		current.Source, current.Mode, len(current.Fields), 100*current.Coverage())

	changes := schema.Diff(baseline, current)
	if len(changes) == 0 {
		fmt.Println("no drift · schema identical")
		return nil
	}

	var breaking, notes []schema.Change
	for _, change := range changes {
		if change.Note {
			notes = append(notes, change)
			continue
		}
		breaking = append(breaking, change)
	}
	for _, change := range breaking {
		fmt.Println(change.String())
	}
	for _, change := range notes {
		fmt.Println(change.String())
	}
	if len(breaking) == 0 {
		// Enum sets follow the traffic, not the format. Reporting them keeps the
		// information; exiting 1 on them would make this command useless on a
		// production log.
		fmt.Printf("no structural drift · %d enum note(s)\n", len(notes))
		return nil
	}
	fmt.Printf("drift detected · %d change(s)\n", len(breaking))
	return errDrift
}
