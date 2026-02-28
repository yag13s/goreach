package merge

import (
	"testing"
	"time"

	"github.com/yag13s/goreach/internal/report"
)

func makeReport(genAt time.Time, funcs map[string]float64) *report.Report {
	functions := make([]report.FuncReport, 0, len(funcs))
	for name, pct := range funcs {
		total := 100
		covered := int(pct)
		functions = append(functions, report.FuncReport{
			Name:              name,
			Line:              10,
			TotalStatements:   total,
			CoveredStatements: covered,
			CoveragePercent:   pct,
		})
	}
	return &report.Report{
		Version:     1,
		GeneratedAt: genAt,
		Mode:        "set",
		Total:       report.CoverageStats{TotalStatements: 100, CoveredStatements: 50, CoveragePercent: 50},
		Packages: []report.PackageReport{
			{
				ImportPath: "example.com/pkg",
				Total:      report.CoverageStats{TotalStatements: 100, CoveredStatements: 50, CoveragePercent: 50},
				Files: []report.FileReport{
					{
						FileName:  "example.com/pkg/foo.go",
						Total:     report.CoverageStats{TotalStatements: 100, CoveredStatements: 50, CoveragePercent: 50},
						Functions: functions,
					},
				},
			},
		},
	}
}

func findFunc(r *report.Report, name string) *report.FuncReport {
	for _, pkg := range r.Packages {
		for _, file := range pkg.Files {
			for i := range file.Functions {
				if file.Functions[i].Name == name {
					return &file.Functions[i]
				}
			}
		}
	}
	return nil
}

func TestMergeMaxCoverage(t *testing.T) {
	old := makeReport(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo": 80,
		"Bar": 20,
	})
	newer := makeReport(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo": 30,
		"Bar": 90,
	})

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}
	if foo.CoveragePercent != 80 {
		t.Errorf("Foo coverage = %v, want 80", foo.CoveragePercent)
	}

	bar := findFunc(merged, "Bar")
	if bar == nil {
		t.Fatal("Bar not found")
	}
	if bar.CoveragePercent != 90 {
		t.Errorf("Bar coverage = %v, want 90", bar.CoveragePercent)
	}
}

func TestMergeNewestBase(t *testing.T) {
	// old has extra function "Legacy" that was deleted
	old := makeReport(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo":    50,
		"Legacy": 0,
	})
	newer := makeReport(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo": 30,
	})

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	// Legacy should be excluded (only in old report)
	if fn := findFunc(merged, "Legacy"); fn != nil {
		t.Error("deleted function Legacy should not appear in merged output")
	}

	// Foo should use max coverage (50 from old)
	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}
	if foo.CoveragePercent != 50 {
		t.Errorf("Foo coverage = %v, want 50", foo.CoveragePercent)
	}
}

func TestMergeNewFuncPreserved(t *testing.T) {
	old := makeReport(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo": 50,
	})
	newer := makeReport(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo":    30,
		"NewFun": 0,
	})

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	fn := findFunc(merged, "NewFun")
	if fn == nil {
		t.Fatal("new function NewFun should be preserved")
	}
	if fn.CoveragePercent != 0 {
		t.Errorf("NewFun coverage = %v, want 0", fn.CoveragePercent)
	}
}

func TestMergeRecomputeStats(t *testing.T) {
	old := makeReport(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo": 100,
	})
	newer := makeReport(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo": 50,
	})

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	// Foo has 100 total, 100 covered → 100%
	if merged.Total.CoveragePercent != 100 {
		t.Errorf("report total coverage = %v, want 100", merged.Total.CoveragePercent)
	}
	if merged.Packages[0].Total.CoveragePercent != 100 {
		t.Errorf("package total coverage = %v, want 100", merged.Packages[0].Total.CoveragePercent)
	}
	if merged.Packages[0].Files[0].Total.CoveragePercent != 100 {
		t.Errorf("file total coverage = %v, want 100", merged.Packages[0].Files[0].Total.CoveragePercent)
	}
}

func TestMergeMode(t *testing.T) {
	r1 := makeReport(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]float64{"Foo": 10})
	r2 := makeReport(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), map[string]float64{"Foo": 20})

	merged, err := Merge([]*report.Report{r1, r2})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Mode != "merged" {
		t.Errorf("Mode = %q, want %q", merged.Mode, "merged")
	}
}

func TestMergeEmptyReports(t *testing.T) {
	_, err := Merge(nil)
	if err == nil {
		t.Fatal("expected error for nil reports")
	}
	_, err = Merge([]*report.Report{})
	if err == nil {
		t.Fatal("expected error for empty reports")
	}
}

func TestMergeSingleReport(t *testing.T) {
	r := makeReport(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]float64{
		"Foo": 42,
	})

	merged, err := Merge([]*report.Report{r})
	if err != nil {
		t.Fatalf("single report should not error: %v", err)
	}
	if merged.Mode != "merged" {
		t.Errorf("Mode = %q, want %q", merged.Mode, "merged")
	}
	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}
	if foo.CoveragePercent != 42 {
		t.Errorf("Foo coverage = %v, want 42", foo.CoveragePercent)
	}
}

// makeReportWithStatements creates a report where each function has explicit
// TotalStatements and CoveredStatements values.
func makeReportWithStatements(genAt time.Time, funcs []report.FuncReport) *report.Report {
	return &report.Report{
		Version:     1,
		GeneratedAt: genAt,
		Mode:        "set",
		Packages: []report.PackageReport{
			{
				ImportPath: "example.com/pkg",
				Files: []report.FileReport{
					{
						FileName:  "example.com/pkg/foo.go",
						Functions: funcs,
					},
				},
			},
		},
	}
}

func TestMerge_ZeroStatementReconcile(t *testing.T) {
	// Old build (from covdata func): has higher coverage but TotalStatements=0
	old := makeReportWithStatements(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{Name: "Foo", CoveragePercent: 80, TotalStatements: 0, CoveredStatements: 0, Line: 0},
		},
	)

	// Newest build (from AST analysis): has real statement counts
	newer := makeReportWithStatements(
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{Name: "Foo", CoveragePercent: 30, TotalStatements: 100, CoveredStatements: 30, Line: 42},
		},
	)

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}

	// Old build wins on coverage (80% > 30%)
	if foo.CoveragePercent != 80 {
		t.Errorf("CoveragePercent = %v, want 80", foo.CoveragePercent)
	}
	// TotalStatements should be restored from base (newer)
	if foo.TotalStatements != 100 {
		t.Errorf("TotalStatements = %v, want 100", foo.TotalStatements)
	}
	// CoveredStatements should be recomputed: round(100 * 80 / 100) = 80
	if foo.CoveredStatements != 80 {
		t.Errorf("CoveredStatements = %v, want 80", foo.CoveredStatements)
	}
	// Line should be restored from base (current source)
	if foo.Line != 42 {
		t.Errorf("Line = %v, want 42", foo.Line)
	}

	// Verify recomputed stats propagate correctly
	if merged.Total.TotalStatements != 100 {
		t.Errorf("Total.TotalStatements = %v, want 100", merged.Total.TotalStatements)
	}
	if merged.Total.CoveredStatements != 80 {
		t.Errorf("Total.CoveredStatements = %v, want 80", merged.Total.CoveredStatements)
	}
}

func TestMerge_ZeroStatementLowerCoverage(t *testing.T) {
	// Old build (from covdata func): lower coverage, TotalStatements=0
	old := makeReportWithStatements(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{Name: "Foo", CoveragePercent: 20, TotalStatements: 0, CoveredStatements: 0, Line: 0},
		},
	)

	// Newest build (from AST analysis): higher coverage wins
	newer := makeReportWithStatements(
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{Name: "Foo", CoveragePercent: 80, TotalStatements: 100, CoveredStatements: 80, Line: 42},
		},
	)

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}

	// Base (newer) wins with 80%, no reconciliation needed
	if foo.CoveragePercent != 80 {
		t.Errorf("CoveragePercent = %v, want 80", foo.CoveragePercent)
	}
	if foo.TotalStatements != 100 {
		t.Errorf("TotalStatements = %v, want 100 (should keep winner's value)", foo.TotalStatements)
	}
	if foo.CoveredStatements != 80 {
		t.Errorf("CoveredStatements = %v, want 80", foo.CoveredStatements)
	}
}

func TestMerge_LatestUnreachedBlocks(t *testing.T) {
	// Old build (covdata func): higher coverage, no unreached blocks
	old := makeReportWithStatements(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{
				Name:              "Foo",
				CoveragePercent:   80,
				TotalStatements:   0,
				CoveredStatements: 0,
				UnreachedBlocks:   nil, // covdata func doesn't produce these
			},
		},
	)

	// Newest build (AST analysis): lower coverage, has unreached blocks
	newer := makeReportWithStatements(
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{
				Name:              "Foo",
				CoveragePercent:   30,
				TotalStatements:   100,
				CoveredStatements: 30,
				UnreachedBlocks: []report.UnreachedBlock{
					{StartLine: 10, EndLine: 15, NumStatements: 3},
					{StartLine: 20, EndLine: 22, NumStatements: 2},
				},
			},
		},
	)

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}

	// Old build wins on coverage (80% > 30%)
	if foo.CoveragePercent != 80 {
		t.Errorf("CoveragePercent = %v, want 80", foo.CoveragePercent)
	}

	// UnreachedBlocks should be nil (from winning old build)
	if foo.UnreachedBlocks != nil {
		t.Errorf("UnreachedBlocks = %v, want nil", foo.UnreachedBlocks)
	}

	// LatestUnreachedBlocks should be set from base (newest build)
	if len(foo.LatestUnreachedBlocks) != 2 {
		t.Fatalf("LatestUnreachedBlocks len = %d, want 2", len(foo.LatestUnreachedBlocks))
	}
	if foo.LatestUnreachedBlocks[0].StartLine != 10 || foo.LatestUnreachedBlocks[0].EndLine != 15 {
		t.Errorf("LatestUnreachedBlocks[0] = %+v, want StartLine=10, EndLine=15", foo.LatestUnreachedBlocks[0])
	}
	if foo.LatestUnreachedBlocks[1].StartLine != 20 || foo.LatestUnreachedBlocks[1].EndLine != 22 {
		t.Errorf("LatestUnreachedBlocks[1] = %+v, want StartLine=20, EndLine=22", foo.LatestUnreachedBlocks[1])
	}
}

func TestMerge_LatestUnreachedBlocks_NewestWins(t *testing.T) {
	// Old build: lower coverage
	old := makeReportWithStatements(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{
				Name:              "Foo",
				CoveragePercent:   30,
				TotalStatements:   100,
				CoveredStatements: 30,
				UnreachedBlocks:   nil,
			},
		},
	)

	// Newest build wins with higher coverage and has unreached blocks
	newer := makeReportWithStatements(
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{
				Name:              "Foo",
				CoveragePercent:   80,
				TotalStatements:   100,
				CoveredStatements: 80,
				UnreachedBlocks: []report.UnreachedBlock{
					{StartLine: 10, EndLine: 12, NumStatements: 2},
				},
			},
		},
	)

	merged, err := Merge([]*report.Report{old, newer})
	if err != nil {
		t.Fatal(err)
	}

	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}

	// Newest build wins (80% > 30%), so UnreachedBlocks are from the winner
	if len(foo.UnreachedBlocks) != 1 {
		t.Errorf("UnreachedBlocks len = %d, want 1", len(foo.UnreachedBlocks))
	}

	// LatestUnreachedBlocks should be nil (newest build won, no need)
	if foo.LatestUnreachedBlocks != nil {
		t.Errorf("LatestUnreachedBlocks = %v, want nil", foo.LatestUnreachedBlocks)
	}
}

func TestMerge_SingleReport_NoLatestBlocks(t *testing.T) {
	r := makeReportWithStatements(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[]report.FuncReport{
			{
				Name:              "Foo",
				CoveragePercent:   50,
				TotalStatements:   100,
				CoveredStatements: 50,
				UnreachedBlocks: []report.UnreachedBlock{
					{StartLine: 10, EndLine: 15, NumStatements: 3},
				},
			},
		},
	)

	merged, err := Merge([]*report.Report{r})
	if err != nil {
		t.Fatal(err)
	}

	foo := findFunc(merged, "Foo")
	if foo == nil {
		t.Fatal("Foo not found")
	}

	// Single report: should not have LatestUnreachedBlocks
	if foo.LatestUnreachedBlocks != nil {
		t.Errorf("LatestUnreachedBlocks = %v, want nil", foo.LatestUnreachedBlocks)
	}

	// UnreachedBlocks should be preserved via deepCopy
	if len(foo.UnreachedBlocks) != 1 {
		t.Errorf("UnreachedBlocks len = %d, want 1", len(foo.UnreachedBlocks))
	}
}
