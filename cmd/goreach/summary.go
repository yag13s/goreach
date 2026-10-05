package main

import (
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"golang.org/x/tools/cover"

	"github.com/yag13s/goreach/internal/covparse"
	"github.com/yag13s/goreach/internal/report"
)

func runSummary(args []string) error {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	reportPath := fs.String("report", "", "path to report.json (as written by analyze or merge)")
	coverDir := fs.String("coverdir", "", "GOCOVERDIR path")
	recursive := fs.Bool("r", false, "recursively search -coverdir for coverage data")
	profilePath := fs.String("profile", "", "path to text coverage profile file")
	_ = fs.Parse(args) // ExitOnError: never returns error

	// positional fallback: goreach summary report.json
	if *reportPath == "" && fs.NArg() > 0 {
		*reportPath = fs.Arg(0)
	}

	if *reportPath != "" {
		if *profilePath != "" || *coverDir != "" {
			return fmt.Errorf("a report cannot be combined with -profile or -coverdir")
		}
		rpt, err := report.ReadFile(*reportPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", *reportPath, err)
		}
		return printSummary(os.Stdout, summarizeReport(rpt))
	}

	if *profilePath == "" && *coverDir == "" {
		return fmt.Errorf("a report, -profile or -coverdir is required")
	}

	var profiles []*cover.Profile
	var err error
	switch {
	case *profilePath != "":
		profiles, err = covparse.ParseProfileFile(*profilePath)
	case *recursive:
		// Raw coverage data from different builds cannot be combined without
		// the source, so only the newest build is summarized.
		var groups []covparse.BuildGroup
		groups, err = covparse.FindBuildGroups(*coverDir)
		if err == nil && len(groups) > 0 {
			if len(groups) > 1 {
				fmt.Fprintf(os.Stderr, "goreach summary: found %d builds, summarizing the newest only; "+
					"use `goreach analyze -r` and `goreach summary <report.json>` for coverage merged across builds\n", len(groups))
			}
			profiles, err = groups[len(groups)-1].Profiles()
		}
	default:
		profiles, err = covparse.ParseDir(*coverDir)
	}
	if err != nil {
		return err
	}

	return printSummary(os.Stdout, summarizeProfiles(profiles))
}

// summaryRow is one line of the summary: a package and its statement counts.
type summaryRow struct {
	pkg            string
	covered, total int
}

// summarizeProfiles totals the statements of raw coverage profiles per
// package. It needs no source code.
func summarizeProfiles(profiles []*cover.Profile) []summaryRow {
	byPkg := make(map[string]summaryRow)
	for _, p := range profiles {
		pkg := path.Dir(p.FileName)
		row := byPkg[pkg]
		row.pkg = pkg
		for _, b := range p.Blocks {
			row.total += b.NumStmt
			if b.Count > 0 {
				row.covered += b.NumStmt
			}
		}
		byPkg[pkg] = row
	}
	return sortedRows(slices.Collect(maps.Values(byPkg)))
}

// summarizeReport returns the per-package totals recorded in a report, so the
// summary shows exactly the numbers analyze, merge and view work with.
func summarizeReport(rpt *report.Report) []summaryRow {
	rows := make([]summaryRow, 0, len(rpt.Packages))
	for _, pkg := range rpt.Packages {
		rows = append(rows, summaryRow{
			pkg:     pkg.ImportPath,
			covered: pkg.Total.CoveredStatements,
			total:   pkg.Total.TotalStatements,
		})
	}
	return sortedRows(rows)
}

// sortedRows sorts rows by package and returns them.
func sortedRows(rows []summaryRow) []summaryRow {
	slices.SortFunc(rows, func(a, b summaryRow) int { return strings.Compare(a.pkg, b.pkg) })
	return rows
}

func printSummary(w io.Writer, rows []summaryRow) error {
	var b strings.Builder
	b.WriteString("Coverage Summary\n")
	b.WriteString("================\n\n")

	var total, covered int
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-60s %5.1f%% (%d/%d)\n", r.pkg, report.ComputePercent(r.covered, r.total), r.covered, r.total)
		total += r.total
		covered += r.covered
	}

	fmt.Fprintf(&b, "\n  %-60s %5.1f%% (%d/%d)\n", "TOTAL", report.ComputePercent(covered, total), covered, total)

	_, err := io.WriteString(w, b.String())
	return err
}
