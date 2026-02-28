package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yag13s/goreach/internal/merge"
	"github.com/yag13s/goreach/internal/report"
	"github.com/yag13s/goreach/internal/viewer"
)

func runMerge(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	outputFile := fs.String("o", "", "output file (default: stdout)")
	pretty := fs.Bool("pretty", false, "pretty-print JSON output")
	_ = fs.Parse(args) // ExitOnError: never returns error

	paths := fs.Args()
	if len(paths) == 0 {
		return fmt.Errorf("at least one report.json path is required")
	}

	reports := make([]*report.Report, 0, len(paths))
	for _, p := range paths {
		r, err := report.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		reports = append(reports, r)
	}

	merged, err := merge.Merge(reports)
	if err != nil {
		return err
	}

	w := os.Stdout
	if *outputFile != "" {
		f, err := os.Create(*outputFile)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	return merged.Write(w, *pretty)
}

func runView(args []string) error {
	fs := flag.NewFlagSet("view", flag.ExitOnError)
	reportPath := fs.String("report", "", "path to report.json")
	port := fs.Int("port", 0, "HTTP port (0 = random available)")
	noOpen := fs.Bool("no-open", false, "do not auto-open browser")
	srcDir := fs.String("src", "", "source root directory for code preview")
	_ = fs.Parse(args) // ExitOnError: never returns error

	// positional fallback: goreach view report.json
	path := *reportPath
	if path == "" && fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	if path == "" {
		return fmt.Errorf("report path required")
	}

	opts := viewer.Options{Port: *port, NoOpen: *noOpen}

	if *srcDir != "" {
		abs, err := filepath.Abs(*srcDir)
		if err != nil {
			return fmt.Errorf("resolve -src path: %w", err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("-src directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("-src %q is not a directory", abs)
		}
		opts.SrcDir = abs
	}

	return viewer.Serve(path, opts)
}
