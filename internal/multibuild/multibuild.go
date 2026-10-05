// Package multibuild analyzes coverage data collected from several builds of
// the same service and folds it into a single report.
//
// Only the newest build can be matched against the source on disk, so it
// alone gets full AST analysis and defines the report's structure. Older
// builds contribute their per-function coverage percentage, obtained from
// `go tool covdata func`, which needs no source.
package multibuild

import (
	"cmp"
	"path"
	"slices"

	"github.com/yag13s/goreach/internal/analysis"
	"github.com/yag13s/goreach/internal/covparse"
	"github.com/yag13s/goreach/internal/merge"
	"github.com/yag13s/goreach/internal/report"
)

// Analyze produces a report from build groups ordered oldest to newest, as
// returned by [covparse.FindBuildGroups]. groups must not be empty.
//
// With a single group the result is the plain analysis of that build.
func Analyze(groups []covparse.BuildGroup, opts analysis.Options) (*report.Report, error) {
	newest, older := groups[len(groups)-1], groups[:len(groups)-1]

	profiles, err := newest.Profiles()
	if err != nil {
		return nil, err
	}
	newestReport, err := analysis.Run(profiles, opts)
	if err != nil {
		return nil, err
	}
	if len(older) == 0 {
		return newestReport, nil
	}

	olderFuncs := make([][]covparse.FuncCoverage, 0, len(older))
	for _, g := range older {
		funcs, err := covparse.RunCovdataFunc(g.Dirs)
		if err != nil {
			return nil, err
		}
		olderFuncs = append(olderFuncs, funcs)
	}
	return combine(newestReport, olderFuncs), nil
}

// combine merges the per-function coverage of older builds onto the newest
// build's report.
func combine(newest *report.Report, older [][]covparse.FuncCoverage) *report.Report {
	olderReports := make([]*report.Report, 0, len(older))
	for _, funcs := range older {
		olderReports = append(olderReports, reportFromFuncCoverage(funcs))
	}
	return merge.Onto(newest, olderReports...)
}

// reportFromFuncCoverage builds a minimal Report from covdata func output.
// TotalStatements/CoveredStatements are left at 0 since covdata func only
// provides a coverage percentage; the merge step reconstructs them from the
// newest build's statement counts.
//
// No package filter is applied here: merging keeps only functions present in
// the newest build's report, which is already filtered.
func reportFromFuncCoverage(funcs []covparse.FuncCoverage) *report.Report {
	byFile := make(map[string][]report.FuncReport)
	for _, fc := range funcs {
		byFile[fc.FileName] = append(byFile[fc.FileName], report.FuncReport{
			Name:            fc.FuncName,
			CoveragePercent: fc.CoveragePercent,
		})
	}

	// Order by package, then file, so a package's files are contiguous.
	fileNames := make([]string, 0, len(byFile))
	for name := range byFile {
		fileNames = append(fileNames, name)
	}
	slices.SortFunc(fileNames, func(a, b string) int {
		return cmp.Or(cmp.Compare(path.Dir(a), path.Dir(b)), cmp.Compare(a, b))
	})

	var pkgs []report.PackageReport
	for _, name := range fileNames {
		importPath := path.Dir(name)
		if len(pkgs) == 0 || pkgs[len(pkgs)-1].ImportPath != importPath {
			pkgs = append(pkgs, report.PackageReport{ImportPath: importPath})
		}
		pkg := &pkgs[len(pkgs)-1]
		pkg.Files = append(pkg.Files, report.FileReport{
			FileName:  name,
			Functions: byFile[name],
		})
	}

	return &report.Report{
		Version:  1,
		Mode:     "covdata-func",
		Packages: pkgs,
	}
}
