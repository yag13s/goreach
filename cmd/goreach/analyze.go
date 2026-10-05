package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/yag13s/goreach/internal/analysis"
	"github.com/yag13s/goreach/internal/covparse"
	"github.com/yag13s/goreach/internal/multibuild"
	"github.com/yag13s/goreach/internal/report"
)

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	profilePath := fs.String("profile", "", "path to text coverage profile file")
	coverDir := fs.String("coverdir", "", "GOCOVERDIR path (mutually exclusive with -profile)")
	recursive := fs.Bool("r", false, "recursively search -coverdir for coverage data")
	pkgFilter := fs.String("pkg", "", "package filter (comma-separated import path prefixes)")
	threshold := fs.Float64("threshold", 100, "show functions with coverage below this percentage")
	minStmts := fs.Int("min-statements", 0, "show functions with at least N unreached statements")
	outputFile := fs.String("o", "", "output file (default: stdout)")
	pretty := fs.Bool("pretty", false, "pretty-print JSON output")
	_ = fs.Parse(args) // ExitOnError: never returns error

	if *profilePath == "" && *coverDir == "" {
		return fmt.Errorf("either -profile or -coverdir is required")
	}
	if *profilePath != "" && *coverDir != "" {
		return fmt.Errorf("-profile and -coverdir are mutually exclusive")
	}
	if *recursive && *coverDir == "" {
		return fmt.Errorf("-r requires -coverdir")
	}
	if *threshold < 0 || *threshold > 100 {
		return fmt.Errorf("-threshold must be between 0 and 100")
	}
	if *minStmts < 0 {
		return fmt.Errorf("-min-statements must be non-negative")
	}

	var prefixes []string
	if *pkgFilter != "" {
		prefixes = strings.Split(*pkgFilter, ",")
	}

	opts := analysis.Options{
		PkgPrefixes:   prefixes,
		Threshold:     *threshold,
		MinStatements: *minStmts,
	}

	var rpt *report.Report
	var err error

	switch {
	case *recursive:
		groups, groupErr := covparse.FindBuildGroups(*coverDir)
		if groupErr != nil {
			return groupErr
		}
		rpt, err = multibuild.Analyze(groups, opts)
	case *profilePath != "":
		profiles, parseErr := covparse.ParseProfileFile(*profilePath)
		if parseErr != nil {
			return parseErr
		}
		rpt, err = analysis.Run(profiles, opts)
	default:
		profiles, parseErr := covparse.ParseDir(*coverDir)
		if parseErr != nil {
			return parseErr
		}
		rpt, err = analysis.Run(profiles, opts)
	}
	if err != nil {
		return err
	}
	rpt.GeneratedAt = time.Now().UTC()

	return writeReport(rpt, *outputFile, *pretty)
}
