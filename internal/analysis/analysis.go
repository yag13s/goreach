// Package analysis matches coverage profiles against source AST to identify unreached code.
package analysis

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/cover"

	"github.com/yag13s/goreach/internal/astmap"
	"github.com/yag13s/goreach/internal/report"
)

// Options controls analysis behavior.
type Options struct {
	// PkgPrefixes filters to only packages matching these import path prefixes.
	// Empty means include all.
	PkgPrefixes []string

	// Threshold filters functions with coverage below this percentage.
	// Default 100 means all functions are included.
	Threshold float64

	// MinStatements filters functions with at least this many unreached statements.
	MinStatements int
}

// Run performs the full analysis pipeline: parse profiles, resolve sources,
// extract AST, match coverage blocks, and return a report.
func Run(profiles []*cover.Profile, opts Options) (*report.Report, error) {
	// Group profiles by package (directory)
	pkgFiles := groupByPackage(profiles)

	// Resolve package import paths to disk paths
	pkgPaths, err := resolvePackages(pkgFiles)
	if err != nil {
		return nil, err
	}

	var pkgReports []report.PackageReport
	var totalStmts, totalCovered int

	// Sort package import paths for deterministic output
	importPaths := make([]string, 0, len(pkgFiles))
	for ip := range pkgFiles {
		importPaths = append(importPaths, ip)
	}
	sort.Strings(importPaths)

	for _, importPath := range importPaths {
		profs := pkgFiles[importPath]
		if !matchesPrefixes(importPath, opts.PkgPrefixes) {
			continue
		}

		diskDir, ok := pkgPaths[importPath]
		if !ok {
			continue
		}

		pkgReport := analyzePackage(importPath, diskDir, profs, opts)
		if pkgReport == nil {
			continue
		}

		totalStmts += pkgReport.Total.TotalStatements
		totalCovered += pkgReport.Total.CoveredStatements
		pkgReports = append(pkgReports, *pkgReport)
	}

	mode := "set"
	if len(profiles) > 0 {
		mode = profiles[0].Mode
	}

	return &report.Report{
		Version: 1,
		Mode:    mode,
		Total: report.CoverageStats{
			TotalStatements:   totalStmts,
			CoveredStatements: totalCovered,
			CoveragePercent:   report.ComputePercent(totalCovered, totalStmts),
		},
		Packages: pkgReports,
	}, nil
}

func analyzePackage(importPath, diskDir string, profiles []*cover.Profile, opts Options) *report.PackageReport {
	var fileReports []report.FileReport
	var pkgStmts, pkgCovered int

	// Sort profiles by filename for deterministic output
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].FileName < profiles[j].FileName
	})

	for _, prof := range profiles {
		baseName := filepath.Base(prof.FileName)
		srcPath := filepath.Join(diskDir, baseName)

		funcs, err := astmap.FileFuncs(srcPath)
		if err != nil {
			continue
		}

		fileReport := analyzeFile(prof, funcs, opts)
		if fileReport == nil {
			continue
		}
		fileReport.FileName = prof.FileName

		pkgStmts += fileReport.Total.TotalStatements
		pkgCovered += fileReport.Total.CoveredStatements
		fileReports = append(fileReports, *fileReport)
	}

	if len(fileReports) == 0 {
		return nil
	}

	return &report.PackageReport{
		ImportPath: importPath,
		Total: report.CoverageStats{
			TotalStatements:   pkgStmts,
			CoveredStatements: pkgCovered,
			CoveragePercent:   report.ComputePercent(pkgCovered, pkgStmts),
		},
		Files: fileReports,
	}
}

func analyzeFile(prof *cover.Profile, funcs []*astmap.FuncExtent, opts Options) *report.FileReport {
	var funcReports []report.FuncReport
	var fileStmts, fileCovered int

	for _, fn := range funcs {
		var totalStmts, coveredStmts int
		var unreached []report.UnreachedBlock

		for _, block := range prof.Blocks {
			if !blockOverlapsFunc(block, fn) {
				continue
			}
			totalStmts += block.NumStmt
			if block.Count > 0 {
				coveredStmts += block.NumStmt
			} else {
				unreached = append(unreached, report.UnreachedBlock{
					StartLine:     block.StartLine,
					StartCol:      block.StartCol,
					EndLine:       block.EndLine,
					EndCol:        block.EndCol,
					NumStatements: block.NumStmt,
				})
			}
		}

		if totalStmts == 0 {
			continue
		}

		pct := report.ComputePercent(coveredStmts, totalStmts)
		unreachedStmts := totalStmts - coveredStmts

		// Apply filters
		if pct > opts.Threshold {
			// Still count towards file totals
			fileStmts += totalStmts
			fileCovered += coveredStmts
			continue
		}
		if unreachedStmts < opts.MinStatements {
			fileStmts += totalStmts
			fileCovered += coveredStmts
			continue
		}

		fileStmts += totalStmts
		fileCovered += coveredStmts

		funcReports = append(funcReports, report.FuncReport{
			Name:              fn.Name,
			Line:              fn.StartLine,
			TotalStatements:   totalStmts,
			CoveredStatements: coveredStmts,
			CoveragePercent:   pct,
			UnreachedBlocks:   unreached,
		})
	}

	if fileStmts == 0 {
		return nil
	}

	return &report.FileReport{
		Total: report.CoverageStats{
			TotalStatements:   fileStmts,
			CoveredStatements: fileCovered,
			CoveragePercent:   report.ComputePercent(fileCovered, fileStmts),
		},
		Functions: funcReports,
	}
}

// blockOverlapsFunc returns true if the coverage block falls within the function's range.
func blockOverlapsFunc(block cover.ProfileBlock, fn *astmap.FuncExtent) bool {
	// Block starts after function ends
	if block.StartLine > fn.EndLine {
		return false
	}
	if block.StartLine == fn.EndLine && block.StartCol > fn.EndCol {
		return false
	}
	// Block ends before function starts
	if block.EndLine < fn.StartLine {
		return false
	}
	if block.EndLine == fn.StartLine && block.EndCol < fn.StartCol {
		return false
	}
	return true
}

// groupByPackage groups profiles by their package import path (directory portion of FileName).
func groupByPackage(profiles []*cover.Profile) map[string][]*cover.Profile {
	m := make(map[string][]*cover.Profile)
	for _, p := range profiles {
		pkg := packageFromFile(p.FileName)
		m[pkg] = append(m[pkg], p)
	}
	return m
}

// packageFromFile extracts the package import path from a coverage profile filename.
// Profile filenames look like "myapp/internal/auth/oauth.go".
func packageFromFile(filename string) string {
	dir := filepath.Dir(filename)
	// Normalize to forward slashes (import paths use /)
	return filepath.ToSlash(dir)
}

// resolvePackages uses `go list -json` to map import paths to disk directories.
func resolvePackages(pkgFiles map[string][]*cover.Profile) (map[string]string, error) {
	importPaths := make([]string, 0, len(pkgFiles))
	for ip := range pkgFiles {
		importPaths = append(importPaths, ip)
	}

	if len(importPaths) == 0 {
		return nil, nil
	}

	args := append([]string{"list", "-json"}, importPaths...)
	cmd := exec.Command("go", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("analysis: go list: %w", err)
	}

	result := make(map[string]string)
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var pkg struct {
			ImportPath string `json:"ImportPath"`
			Dir        string `json:"Dir"`
		}
		if err := dec.Decode(&pkg); err != nil {
			break
		}
		result[pkg.ImportPath] = pkg.Dir
	}
	return result, nil
}

// matchesPrefixes returns true if importPath matches any of the given prefixes,
// or if prefixes is empty (match all).
func matchesPrefixes(importPath string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		if strings.HasPrefix(importPath, p) {
			return true
		}
	}
	return false
}
