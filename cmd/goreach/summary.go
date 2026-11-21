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
	top := fs.Int("top", 0, "list the N most called functions instead of the package summary (needs a report)")
	loops := fs.Int("loops", 0, "list the N calls made inside loops that ran most, with how often per call of their caller: N+1 candidates (needs a report)")
	cold := fs.Bool("cold", false, "list the functions that were never called instead of the package summary (needs a report)")
	pkgFilter := fs.String("pkg", "", "with -top/-loops/-cold: package filter (comma-separated import path prefixes)")
	_ = fs.Parse(args) // ExitOnError: never returns error

	rptPath, err := reportPathArg(fs, *reportPath)
	if err != nil {
		return err
	}
	if *recursive && *coverDir == "" {
		return fmt.Errorf("-r requires -coverdir")
	}
	if *top < 0 || *loops < 0 {
		return fmt.Errorf("-top and -loops must be non-negative")
	}
	byCalls := *top > 0 || *loops > 0 || *cold
	if *pkgFilter != "" && !byCalls {
		return fmt.Errorf("-pkg applies to -top, -loops and -cold only")
	}
	if byCalls && rptPath == "" {
		return fmt.Errorf("-top, -loops and -cold need a report: run `goreach analyze -o report.json` first")
	}

	if rptPath != "" {
		if *profilePath != "" || *coverDir != "" {
			return fmt.Errorf("a report cannot be combined with -profile or -coverdir")
		}
		rpt, err := report.ReadFile(rptPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", rptPath, err)
		}
		if byCalls {
			var prefixes []string
			if *pkgFilter != "" {
				prefixes = strings.Split(*pkgFilter, ",")
			}
			return printCalls(os.Stdout, rpt, prefixes, callsView{top: *top, loops: *loops, cold: *cold})
		}
		return printSummary(os.Stdout, summarizeReport(rpt))
	}

	if *profilePath == "" && *coverDir == "" {
		return fmt.Errorf("a report, -profile or -coverdir is required")
	}
	if *profilePath != "" && *coverDir != "" {
		return fmt.Errorf("-profile and -coverdir are mutually exclusive")
	}

	var profiles []*cover.Profile
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
