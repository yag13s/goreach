// Package analysis matches coverage profiles against source AST to identify unreached code.
package analysis

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
}

// Run performs the full analysis pipeline: parse profiles, resolve sources,
// extract AST, match coverage blocks, and return a report.
func Run(profiles []*cover.Profile, opts Options) (*report.Report, error) {
	// Group profiles by package (directory)
	pkgFiles := groupByPackage(profiles)

	// Sort package import paths for deterministic output
	importPaths := slices.Sorted(maps.Keys(pkgFiles))

	// Resolve package import paths to disk paths
	pkgPaths, err := resolvePackages(importPaths)
	if err != nil {
		return nil, err
	}

	var pkgReports []report.PackageReport
	var totalStmts, totalCovered int

	for _, importPath := range importPaths {
		profs := pkgFiles[importPath]
		if !matchesPrefixes(importPath, opts.PkgPrefixes) {
			continue
		}

		diskDir, ok := pkgPaths[importPath]
		if !ok {
			fmt.Fprintf(os.Stderr, "goreach: warning: package %s not found by go list, skipping\n", importPath)
			continue
		}

		pkgReport := analyzePackage(importPath, diskDir, profs)
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
		Version:  report.SchemaVersion,
		Mode:     mode,
		Total:    report.NewCoverageStats(totalCovered, totalStmts),
		Packages: pkgReports,
	}, nil
}

func analyzePackage(importPath, diskDir string, profiles []*cover.Profile) *report.PackageReport {
	var fileReports []report.FileReport
	var pkgStmts, pkgCovered int

	// Sort profiles by filename for deterministic output
	slices.SortFunc(profiles, func(a, b *cover.Profile) int {
		return strings.Compare(a.FileName, b.FileName)
	})

	for _, prof := range profiles {
		baseName := filepath.Base(prof.FileName)
		srcPath := filepath.Join(diskDir, baseName)

		funcs, err := astmap.FileFuncs(srcPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goreach: warning: %v\n", err)
			continue
		}

		fileReport := analyzeFile(prof, funcs)
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
		Total:      report.NewCoverageStats(pkgCovered, pkgStmts),
		Files:      fileReports,
	}
}

// analyzeFile attributes the profile's blocks to the file's functions.
//
// The file totals are those of the profile: every block counts, including
// blocks outside any function declaration (such as a function literal in a
// package-level var), so they can exceed the sum over Functions.
func analyzeFile(prof *cover.Profile, funcs []*astmap.FuncExtent) *report.FileReport {
	var fileStmts, fileCovered int
	for _, block := range prof.Blocks {
		fileStmts += block.NumStmt
		if block.Count > 0 {
			fileCovered += block.NumStmt
		}
	}
	if fileStmts == 0 {
		return nil
	}

	funcReports := make([]report.FuncReport, 0, len(funcs))
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

		funcReports = append(funcReports, report.FuncReport{
			Name:              fn.Name,
			Line:              fn.StartLine,
			TotalStatements:   totalStmts,
			CoveredStatements: coveredStmts,
			CoveragePercent:   report.ComputePercent(coveredStmts, totalStmts),
			UnreachedBlocks:   unreached,
		})
	}

	return &report.FileReport{
		Total:     report.NewCoverageStats(fileCovered, fileStmts),
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
func resolvePackages(importPaths []string) (map[string]string, error) {
	if len(importPaths) == 0 {
		return nil, nil
	}

	args := append([]string{"list", "-json"}, importPaths...)
	cmd := exec.Command("go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("analysis: go list: %w\n%s", err, bytes.TrimSpace(stderr.Bytes()))
	}

	result := make(map[string]string)
	// go list prints one JSON object per package, back to back.
	dec := jsontext.NewDecoder(bytes.NewReader(out))
	for {
		var pkg struct {
			ImportPath string `json:"ImportPath"`
			Dir        string `json:"Dir"`
		}
		err := json.UnmarshalDecode(dec, &pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("analysis: decode go list output: %w", err)
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
