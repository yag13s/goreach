// Package report defines the JSON report schema and generation for goreach.
package report

import (
	"encoding/json"
	"io"
	"time"
)

// Report is the top-level JSON output of goreach analyze.
type Report struct {
	Version     int            `json:"version"`
	GeneratedAt time.Time     `json:"generated_at"`
	Mode        string         `json:"mode"`
	Total       CoverageStats  `json:"total"`
	Packages    []PackageReport `json:"packages"`
}

// CoverageStats holds aggregate coverage statistics.
type CoverageStats struct {
	TotalStatements   int     `json:"total_statements"`
	CoveredStatements int     `json:"covered_statements"`
	CoveragePercent   float64 `json:"coverage_percent"`
}

// PackageReport holds coverage data for a single package.
type PackageReport struct {
	ImportPath string       `json:"import_path"`
	Total      CoverageStats `json:"total"`
	Files      []FileReport  `json:"files"`
}

// FileReport holds coverage data for a single source file.
type FileReport struct {
	FileName  string         `json:"file_name"`
	Total     CoverageStats  `json:"total"`
	Functions []FuncReport   `json:"functions"`
}

// FuncReport holds coverage data for a single function.
type FuncReport struct {
	Name              string          `json:"name"`
	Line              int             `json:"line"`
	TotalStatements   int             `json:"total_statements"`
	CoveredStatements int             `json:"covered_statements"`
	CoveragePercent   float64         `json:"coverage_percent"`
	UnreachedBlocks   []UnreachedBlock `json:"unreached_blocks,omitempty"`
}

// UnreachedBlock describes a contiguous block of unreached code.
type UnreachedBlock struct {
	StartLine     int `json:"start_line"`
	StartCol      int `json:"start_col"`
	EndLine       int `json:"end_line"`
	EndCol        int `json:"end_col"`
	NumStatements int `json:"num_statements"`
}

// Write serializes the report as JSON to the given writer.
func (r *Report) Write(w io.Writer, pretty bool) error {
	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(r)
}

// ComputePercent calculates coverage percentage, returning 0 for zero total.
func ComputePercent(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total) * 100
}
