package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/tools/cover"

	"github.com/yag13s/goreach/internal/analysis"
	"github.com/yag13s/goreach/internal/covparse"
	"github.com/yag13s/goreach/internal/merge"
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
		groups, parseErr := covparse.ParseDirRecursiveGrouped(*coverDir)
		if parseErr != nil {
			return parseErr
		}
		if len(groups) == 1 {
			text, textErr := groups[0].ParseProfile()
			if textErr != nil {
				return textErr
			}
			rpt, err = analyzeProfileText(text, opts)
		} else {
			// Newest build (last element) gets full AST analysis.
			newest := groups[len(groups)-1]
			newestText, textErr := newest.ParseProfile()
			if textErr != nil {
				return textErr
			}
			newestRpt, rErr := analyzeProfileText(newestText, opts)
			if rErr != nil {
				return rErr
			}
			newestRpt.GeneratedAt = time.Now().UTC()

			reports := make([]*report.Report, 0, len(groups))
			// Older builds use covdata func (no AST dependency).
			for _, g := range groups[:len(groups)-1] {
				funcCov, fErr := covparse.RunCovdataFunc(g.Dirs)
				if fErr != nil {
					return fErr
				}
				r := reportFromFuncCoverage(funcCov, opts)
				r.GeneratedAt = g.NewestTimestamp
				reports = append(reports, r)
			}
			reports = append(reports, newestRpt)

			rpt, err = merge.Merge(reports)
		}
	case *profilePath != "":
		profileText, parseErr := covparse.ParseProfileFile(*profilePath)
		if parseErr != nil {
			return parseErr
		}
		rpt, err = analyzeProfileText(profileText, opts)
	default:
		profileText, parseErr := covparse.ParseDir(*coverDir)
		if parseErr != nil {
			return parseErr
		}
		rpt, err = analyzeProfileText(profileText, opts)
	}
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

// analyzeProfileText parses a text coverage profile and runs analysis on it.
func analyzeProfileText(text string, opts analysis.Options) (*report.Report, error) {
	tmpFile, err := os.CreateTemp("", "goreach-analyze-*.txt")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(text); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	profiles, err := cover.ParseProfiles(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}

	return analysis.Run(profiles, opts)
}

// reportFromFuncCoverage builds a minimal Report from covdata func output.
// TotalStatements/CoveredStatements are set to 0 since covdata func only
// provides a coverage percentage. The merge step will reconcile these
// using the base (newest build) report's statement counts.
func reportFromFuncCoverage(funcs []covparse.FuncCoverage, opts analysis.Options) *report.Report {
	// Group by package (directory portion of FileName)
	type fileData struct {
		fileName  string
		functions []report.FuncReport
	}
	type pkgData struct {
		files map[string]*fileData
	}

	pkgs := make(map[string]*pkgData)
	for _, fc := range funcs {
		pkg := filepath.ToSlash(filepath.Dir(fc.FileName))

		if len(opts.PkgPrefixes) > 0 {
			matched := false
			for _, p := range opts.PkgPrefixes {
				if strings.HasPrefix(pkg, p) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		if pkgs[pkg] == nil {
			pkgs[pkg] = &pkgData{files: make(map[string]*fileData)}
		}
		fd := pkgs[pkg].files[fc.FileName]
		if fd == nil {
			fd = &fileData{fileName: fc.FileName}
			pkgs[pkg].files[fc.FileName] = fd
		}
		fd.functions = append(fd.functions, report.FuncReport{
			Name:            fc.FuncName,
			CoveragePercent: fc.CoveragePercent,
		})
	}

	var pkgReports []report.PackageReport
	for importPath, pd := range pkgs {
		var fileReports []report.FileReport
		for _, fd := range pd.files {
			fileReports = append(fileReports, report.FileReport{
				FileName:  fd.fileName,
				Functions: fd.functions,
			})
		}
		pkgReports = append(pkgReports, report.PackageReport{
			ImportPath: importPath,
			Files:      fileReports,
		})
	}

	return &report.Report{
		Version:  1,
		Mode:     "covdata-func",
		Packages: pkgReports,
	}
}
