// Package merge combines multiple goreach report.json files into a single
// report by taking the maximum coverage per function across all builds.
package merge

import (
	"fmt"
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
	line              int
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
		r := deepCopy(reports[0])
		r.GeneratedAt = time.Now().UTC()
		r.Mode = "merged"
		return r, nil
	}

	// Find the newest report to use as the structural base.
	base := reports[0]
	for _, r := range reports[1:] {
		if r.GeneratedAt.After(base.GeneratedAt) {
			base = r
		}
	}

	// Build a lookup of max coverage per function across all reports.
	lookup := make(map[funcKey]*funcEntry)
	for _, r := range reports {
		for _, pkg := range r.Packages {
			for _, file := range pkg.Files {
				for _, fn := range file.Functions {
					key := funcKey{fileName: file.FileName, funcName: fn.Name}
					existing, ok := lookup[key]
					if !ok || fn.CoveragePercent > existing.coveragePercent {
						lookup[key] = &funcEntry{
							coveragePercent:   fn.CoveragePercent,
							coveredStatements: fn.CoveredStatements,
							totalStatements:   fn.TotalStatements,
							unreachedBlocks:   fn.UnreachedBlocks,
							line:              fn.Line,
						}
					}
				}
			}
		}
	}

	// Deep-copy the base report structure and apply best coverage values.
	merged := &report.Report{
		Version:     base.Version,
		GeneratedAt: time.Now().UTC(),
		Mode:        "merged",
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
						Line:              best.line,
						TotalStatements:   best.totalStatements,
						CoveredStatements: best.coveredStatements,
						CoveragePercent:   best.coveragePercent,
						UnreachedBlocks:   best.unreachedBlocks,
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
	return merged, nil
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

			r.Packages[i].Files[j].Total = report.CoverageStats{
				TotalStatements:   fileTotal,
				CoveredStatements: fileCovered,
				CoveragePercent:   report.ComputePercent(fileCovered, fileTotal),
			}
			pkgTotal += fileTotal
			pkgCovered += fileCovered
		}

		r.Packages[i].Total = report.CoverageStats{
			TotalStatements:   pkgTotal,
			CoveredStatements: pkgCovered,
			CoveragePercent:   report.ComputePercent(pkgCovered, pkgTotal),
		}
		reportTotal += pkgTotal
		reportCovered += pkgCovered
	}

	r.Total = report.CoverageStats{
		TotalStatements:   reportTotal,
		CoveredStatements: reportCovered,
		CoveragePercent:   report.ComputePercent(reportCovered, reportTotal),
	}
}

// deepCopy returns a deep copy of the report so the caller can mutate it
// without affecting the original.
func deepCopy(src *report.Report) *report.Report {
	dst := &report.Report{
		Version:     src.Version,
		GeneratedAt: src.GeneratedAt,
		Mode:        src.Mode,
		Total:       src.Total,
		Packages:    make([]report.PackageReport, len(src.Packages)),
	}
	for i, pkg := range src.Packages {
		dp := report.PackageReport{
			ImportPath: pkg.ImportPath,
			Total:      pkg.Total,
			Files:      make([]report.FileReport, len(pkg.Files)),
		}
		for j, file := range pkg.Files {
			df := report.FileReport{
				FileName:  file.FileName,
				Total:     file.Total,
				Functions: make([]report.FuncReport, len(file.Functions)),
			}
			for k, fn := range file.Functions {
				df.Functions[k] = report.FuncReport{
					Name:              fn.Name,
					Line:              fn.Line,
					TotalStatements:   fn.TotalStatements,
					CoveredStatements: fn.CoveredStatements,
					CoveragePercent:   fn.CoveragePercent,
				}
				if len(fn.UnreachedBlocks) > 0 {
					df.Functions[k].UnreachedBlocks = make([]report.UnreachedBlock, len(fn.UnreachedBlocks))
					copy(df.Functions[k].UnreachedBlocks, fn.UnreachedBlocks)
				}
			}
			dp.Files[j] = df
		}
		dst.Packages[i] = dp
	}
	return dst
}
