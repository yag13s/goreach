package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/cover"

	"github.com/yag13s/goreach/internal/covparse"
	"github.com/yag13s/goreach/internal/report"
)

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
		// Use only the newest build group's profile for summary.
		var groups []covparse.BuildGroup
		groups, err = covparse.ParseDirRecursiveGrouped(*coverDir)
		if err == nil && len(groups) > 0 {
			profileText, err = groups[len(groups)-1].ParseProfile()
		}
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
