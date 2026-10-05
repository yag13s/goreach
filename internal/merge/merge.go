// Package merge combines multiple goreach report.json files into a single
// report by taking the maximum coverage per function across all builds.
package merge

import (
	"fmt"
	"math"
	"time"

	"github.com/yag13s/goreach/internal/report"
)

// funcKey uniquely identifies a function across builds (line numbers may shift).
type funcKey struct {
	fileName string
	funcName string
}

// funcEntry tracks the best coverage seen for a function across all reports.
type funcEntry struct {
	coveragePercent   float64
	coveredStatements int
	totalStatements   int
	unreachedBlocks   []report.UnreachedBlock
	fromBase          bool
}

// Merge combines multiple reports into one. It uses the newest report (by
// GeneratedAt) as the structural base and replaces each function's coverage
// with the maximum value observed across all input reports.
//
// Functions that exist only in older reports (i.e. deleted code) are excluded.
// Functions that exist only in the newest report are kept as-is.
func Merge(reports []*report.Report) (*report.Report, error) {
	if len(reports) == 0 {
		return nil, fmt.Errorf("merge requires at least 1 report, got 0")
	}

	// Single report: pass through with updated metadata.
	if len(reports) == 1 {
		r := reports[0].Clone()
		r.GeneratedAt = time.Now().UTC()
		r.Mode = report.ModeMerged
		return r, nil
	}

	// Find the newest report to use as the structural base.
	base := reports[0]
	for _, r := range reports[1:] {
		if r.GeneratedAt.After(base.GeneratedAt) {
			base = r
		}
	}
	others := make([]*report.Report, 0, len(reports)-1)
	for _, r := range reports {
		if r != base {
			others = append(others, r)
		}
	}

	return Onto(base, others...), nil
}

// Onto merges others onto base: the result has base's structure (packages,
// files, functions, line numbers), with each function's coverage replaced by
// the maximum observed across base and others. On a tie, base wins.
//
// Use it instead of [Merge] when the caller already knows which report
// describes the current source.
func Onto(base *report.Report, others ...*report.Report) *report.Report {
	// Build a lookup of max coverage per function across all reports.
	lookup := make(map[funcKey]*funcEntry)
	consider := func(r *report.Report, isBase bool) {
		for _, pkg := range r.Packages {
			for _, file := range pkg.Files {
				for _, fn := range file.Functions {
					key := funcKey{fileName: file.FileName, funcName: fn.Name}
					existing, ok := lookup[key]
					if !ok || fn.CoveragePercent > existing.coveragePercent ||
						(fn.CoveragePercent == existing.coveragePercent && isBase) {
						lookup[key] = &funcEntry{
							coveragePercent:   fn.CoveragePercent,
							coveredStatements: fn.CoveredStatements,
							totalStatements:   fn.TotalStatements,
							unreachedBlocks:   fn.UnreachedBlocks,
							fromBase:          isBase,
						}
					}
				}
			}
		}
	}
	for _, r := range others {
		consider(r, false)
	}
	consider(base, true)

	// Deep-copy the base report structure and apply best coverage values.
	merged := &report.Report{
		Version:     base.Version,
		GeneratedAt: time.Now().UTC(),
		Mode:        report.ModeMerged,
		Packages:    make([]report.PackageReport, len(base.Packages)),
	}

	for i, pkg := range base.Packages {
		mp := report.PackageReport{
			ImportPath: pkg.ImportPath,
			Files:      make([]report.FileReport, len(pkg.Files)),
		}
		for j, file := range pkg.Files {
			mf := report.FileReport{
				FileName:  file.FileName,
				Functions: make([]report.FuncReport, len(file.Functions)),
			}
			for k, fn := range file.Functions {
				key := funcKey{fileName: file.FileName, funcName: fn.Name}
				if best, ok := lookup[key]; ok {
					mf.Functions[k] = report.FuncReport{
						Name:              fn.Name,
						Line:              fn.Line, // always use base (current source) line
						TotalStatements:   best.totalStatements,
						CoveredStatements: best.coveredStatements,
						CoveragePercent:   best.coveragePercent,
						UnreachedBlocks:   best.unreachedBlocks,
					}
					// When an older build won on coverage, its unreached blocks
					// have line numbers from the old source. Preserve the base
					// (latest build) blocks so the viewer can show a toggle
					// between merged coverage and current-source blocks.
					if !best.fromBase && len(fn.UnreachedBlocks) > 0 {
						mf.Functions[k].LatestUnreachedBlocks = fn.UnreachedBlocks
					}
					// Reconcile: if the winner came from covdata func
					// (TotalStatements==0) but the base has real counts,
					// reconstruct statement counts from the base.
					if best.totalStatements == 0 && fn.TotalStatements > 0 {
						total := fn.TotalStatements
						covered := int(math.Round(float64(total) * best.coveragePercent / 100))
						mf.Functions[k].TotalStatements = total
						mf.Functions[k].CoveredStatements = covered
					}
				} else {
					mf.Functions[k] = fn
				}
			}
			mp.Files[j] = mf
		}
		merged.Packages[i] = mp
	}

	recomputeStats(merged)
	return merged
}

// recomputeStats recalculates aggregate statistics bottom-up:
// function → file → package → report total.
func recomputeStats(r *report.Report) {
	var reportTotal, reportCovered int

	for i := range r.Packages {
		var pkgTotal, pkgCovered int

		for j := range r.Packages[i].Files {
			var fileTotal, fileCovered int

			for _, fn := range r.Packages[i].Files[j].Functions {
				fileTotal += fn.TotalStatements
				fileCovered += fn.CoveredStatements
			}

			r.Packages[i].Files[j].Total = report.NewCoverageStats(fileCovered, fileTotal)
			pkgTotal += fileTotal
			pkgCovered += fileCovered
		}

		r.Packages[i].Total = report.NewCoverageStats(pkgCovered, pkgTotal)
		reportTotal += pkgTotal
		reportCovered += pkgCovered
	}

	r.Total = report.NewCoverageStats(reportCovered, reportTotal)
}
