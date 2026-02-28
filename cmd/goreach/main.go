// Command goreach analyzes Go coverage data to identify unreached code paths.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/tools/cover"

	"github.com/yag13s/goreach/internal/analysis"
	"github.com/yag13s/goreach/internal/covparse"
	"github.com/yag13s/goreach/internal/merge"
	"github.com/yag13s/goreach/internal/report"
	"github.com/yag13s/goreach/internal/viewer"
)

var version = "dev"

func init() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("goreach %s\n", version)
	case "analyze":
		if err := runAnalyze(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach analyze: %v\n", err)
			os.Exit(1)
		}
	case "summary":
		if err := runSummary(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach summary: %v\n", err)
			os.Exit(1)
		}
	case "merge":
		if err := runMerge(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach merge: %v\n", err)
			os.Exit(1)
		}
	case "view":
		if err := runView(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach view: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "goreach: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: goreach <command> [flags]

Commands:
  analyze   Analyze coverage data and output JSON report
  merge     Merge multiple report.json files (max coverage per function)
  summary   Print coverage summary as text
  view      Open report.json in browser UI
  version   Print version information`)
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

	// Get profile text
	var profileText string
	var err error
	switch {
	case *profilePath != "":
		profileText, err = covparse.ParseProfileFile(*profilePath)
	case *recursive:
		profileText, err = covparse.ParseDirRecursive(*coverDir)
	default:
		profileText, err = covparse.ParseDir(*coverDir)
	}
	if err != nil {
		return err
	}

	// Write profile to temp file for x/tools/cover parsing
	tmpFile, err := os.CreateTemp("", "goreach-analyze-*.txt")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(profileText); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	profiles, err := cover.ParseProfiles(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("parse profiles: %w", err)
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

	rpt, err := analysis.Run(profiles, opts)
	if err != nil {
		return err
	}
	rpt.GeneratedAt = time.Now().UTC()

	w := os.Stdout
	if *outputFile != "" {
		f, err := os.Create(*outputFile)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	return rpt.Write(w, *pretty)
}

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

func runSummary(args []string) error {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	coverDir := fs.String("coverdir", "", "GOCOVERDIR path")
	recursive := fs.Bool("r", false, "recursively search -coverdir for coverage data")
	profilePath := fs.String("profile", "", "path to text coverage profile file")
	_ = fs.Parse(args) // ExitOnError: never returns error

	if *profilePath == "" && *coverDir == "" {
		return fmt.Errorf("either -profile or -coverdir is required")
	}

	var profileText string
	var err error
	switch {
	case *profilePath != "":
		profileText, err = covparse.ParseProfileFile(*profilePath)
	case *recursive:
		profileText, err = covparse.ParseDirRecursive(*coverDir)
	default:
		profileText, err = covparse.ParseDir(*coverDir)
	}
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp("", "goreach-summary-*.txt")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(profileText); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	profiles, err := cover.ParseProfiles(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("parse profiles: %w", err)
	}

	// Compute summary per package
	type pkgStats struct {
		total, covered int
	}
	stats := make(map[string]*pkgStats)
	var overallTotal, overallCovered int
	for _, p := range profiles {
		pkg := strings.TrimSuffix(p.FileName, "/"+filepath.Base(p.FileName))
		if stats[pkg] == nil {
			stats[pkg] = &pkgStats{}
		}
		for _, b := range p.Blocks {
			stats[pkg].total += b.NumStmt
			overallTotal += b.NumStmt
			if b.Count > 0 {
				stats[pkg].covered += b.NumStmt
				overallCovered += b.NumStmt
			}
		}
	}

	// Print summary
	fmt.Printf("Coverage Summary\n")
	fmt.Printf("================\n\n")

	// Sort packages
	pkgs := make([]string, 0, len(stats))
	for p := range stats {
		pkgs = append(pkgs, p)
	}
	sortStrings(pkgs)

	for _, pkg := range pkgs {
		s := stats[pkg]
		pct := report.ComputePercent(s.covered, s.total)
		fmt.Printf("  %-60s %5.1f%% (%d/%d)\n", pkg, pct, s.covered, s.total)
	}

	fmt.Printf("\n  %-60s %5.1f%% (%d/%d)\n", "TOTAL", report.ComputePercent(overallCovered, overallTotal), overallCovered, overallTotal)
	return nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
