// Package report defines the JSON report schema and generation for goreach.
package report

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"os"
	"slices"
	"time"
)

// SchemaVersion is the value of [Report.Version] written by this package.
const SchemaVersion = 1

// ModeMerged is the [Report.Mode] of a report produced by merging several
// reports. Reports produced directly from coverage data carry the coverage
// mode of the profile instead ("set", "count" or "atomic").
const ModeMerged = "merged"

// Report is the top-level JSON output of goreach analyze.
type Report struct {
	Version     int             `json:"version"`
	GeneratedAt time.Time       `json:"generated_at"`
	Mode        string          `json:"mode"`
	Total       CoverageStats   `json:"total"`
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
	ImportPath string        `json:"import_path"`
	Total      CoverageStats `json:"total"`
	Files      []FileReport  `json:"files"`
}

// FileReport holds coverage data for a single source file.
type FileReport struct {
	FileName  string        `json:"file_name"`
	Total     CoverageStats `json:"total"`
	Functions []FuncReport  `json:"functions"`
}

// FuncReport holds coverage data for a single function.
type FuncReport struct {
	Name                  string           `json:"name"`
	Line                  int              `json:"line"`
	TotalStatements       int              `json:"total_statements"`
	CoveredStatements     int              `json:"covered_statements"`
	CoveragePercent       float64          `json:"coverage_percent"`
	UnreachedBlocks       []UnreachedBlock `json:"unreached_blocks,omitempty"`
	LatestUnreachedBlocks []UnreachedBlock `json:"latest_unreached_blocks,omitempty"`
}

// UnreachedBlock describes a contiguous block of unreached code.
type UnreachedBlock struct {
	StartLine     int `json:"start_line"`
	StartCol      int `json:"start_col"`
	EndLine       int `json:"end_line"`
	EndCol        int `json:"end_col"`
	NumStatements int `json:"num_statements"`
}

// ReadFile reads and deserializes a JSON report from the given file path.
func ReadFile(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r Report
	if err := json.UnmarshalRead(f, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Write serializes the report as JSON to the given writer, followed by a
// newline. Empty lists are written as [], never null, whether or not the
// slice is nil.
func (r *Report) Write(w io.Writer, pretty bool) error {
	var opts []json.Options
	if pretty {
		opts = append(opts, jsontext.WithIndent("  "))
	}
	if err := json.MarshalWrite(w, r, opts...); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// Clone returns a deep copy of the report.
func (r *Report) Clone() *Report {
	c := *r
	c.Packages = slices.Clone(r.Packages)
	for i := range c.Packages {
		pkg := &c.Packages[i]
		pkg.Files = slices.Clone(pkg.Files)
		for j := range pkg.Files {
			file := &pkg.Files[j]
			file.Functions = slices.Clone(file.Functions)
			for k := range file.Functions {
				fn := &file.Functions[k]
				fn.UnreachedBlocks = slices.Clone(fn.UnreachedBlocks)
				fn.LatestUnreachedBlocks = slices.Clone(fn.LatestUnreachedBlocks)
			}
		}
	}
	return &c
}

// FuncFilter selects which functions a report lists.
type FuncFilter struct {
	// MaxCoverage keeps functions whose coverage percentage is at most this
	// value. 100 keeps every function.
	MaxCoverage float64

	// MinUnreached keeps functions with at least this many unreached
	// statements. 0 keeps every function.
	MinUnreached int
}

func (f FuncFilter) keeps(fn FuncReport) bool {
	return fn.CoveragePercent <= f.MaxCoverage &&
		fn.TotalStatements-fn.CoveredStatements >= f.MinUnreached
}

// FilterFunctions removes the functions f does not keep.
//
// Only the function lists change. File, package and report totals keep
// describing all the code, so a filtered report shows the same coverage as
// the unfiltered one. Files left with no functions stay in the report.
func (r *Report) FilterFunctions(f FuncFilter) {
	for i := range r.Packages {
		for j := range r.Packages[i].Files {
			file := &r.Packages[i].Files[j]
			file.Functions = slices.DeleteFunc(file.Functions, func(fn FuncReport) bool {
				return !f.keeps(fn)
			})
		}
	}
}

// NewCoverageStats returns the stats for covered out of total statements.
func NewCoverageStats(covered, total int) CoverageStats {
	return CoverageStats{
		TotalStatements:   total,
		CoveredStatements: covered,
		CoveragePercent:   ComputePercent(covered, total),
	}
}

// ComputePercent calculates coverage percentage, returning 0 for zero total.
func ComputePercent(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total) * 100
}
