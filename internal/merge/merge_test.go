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

func TestOnto_ExplicitBaseIgnoresTimestamps(t *testing.T) {
	// base is older by timestamp, but the caller says it is the current source.
	base := makeReport(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]float64{"Foo": 20})
	base.Packages[0].Files[0].Functions[0].Line = 42
	other := makeReport(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), map[string]float64{"Foo": 80, "Gone": 90})

	got := Onto(base, other)

	if len(got.Packages[0].Files[0].Functions) != 1 {
		t.Fatalf("expected only base's functions, got %+v", got.Packages[0].Files[0].Functions)
	}
	foo := findFunc(got, "Foo")
	if foo.CoveragePercent != 80 {
		t.Errorf("Foo coverage = %v, want 80 (max across reports)", foo.CoveragePercent)
	}
	if foo.Line != 42 {
		t.Errorf("Foo line = %d, want 42 (from base)", foo.Line)
	}
	if got.Mode != "merged" {
		t.Errorf("mode = %q, want merged", got.Mode)
	}
}

func TestOnto_TiePrefersBase(t *testing.T) {
	blocks := []report.UnreachedBlock{{StartLine: 5, EndLine: 6, NumStatements: 1}}
	base := makeReport(time.Time{}, map[string]float64{"Foo": 50})
	base.Packages[0].Files[0].Functions[0].UnreachedBlocks = blocks
	other := makeReport(time.Time{}, map[string]float64{"Foo": 50})

	foo := findFunc(Onto(base, other), "Foo")
	if len(foo.UnreachedBlocks) != 1 || foo.LatestUnreachedBlocks != nil {
		t.Errorf("expected base's blocks to win the tie, got %+v / %+v", foo.UnreachedBlocks, foo.LatestUnreachedBlocks)
	}
}

// reportWithFile builds a report with a single file whose totals are given
// explicitly, so they can exceed what the listed functions add up to.
func reportWithFile(total report.CoverageStats, funcs ...report.FuncReport) *report.Report {
	return &report.Report{
		Version: report.SchemaVersion,
		Mode:    "atomic",
		Total:   total,
		Packages: []report.PackageReport{{
			ImportPath: "example.com/pkg",
			Total:      total,
			Files: []report.FileReport{{
				FileName:  "example.com/pkg/foo.go",
				Total:     total,
				Functions: funcs,
			}},
		}},
	}
}

func TestOnto_TotalsKeepStatementsNotListedInBase(t *testing.T) {
	// The file has 20 statements, 12 covered. Only Cold (2/8) is listed: the
	// other 12 statements (10 covered) belong to functions that were filtered
	// out, or to no function at all.
	base := reportWithFile(report.NewCoverageStats(12, 20),
		report.FuncReport{Name: "Cold", TotalStatements: 8, CoveredStatements: 2, CoveragePercent: 25},
	)
	other := reportWithFile(report.NewCoverageStats(0, 0),
		report.FuncReport{Name: "Cold", CoveragePercent: 75},
	)

	got := Onto(base, other)

	// Cold improves from 2/8 to 6/8; the unlisted 10/12 is carried over.
	want := report.NewCoverageStats(16, 20)
	if got.Total != want || got.Packages[0].Total != want || got.Packages[0].Files[0].Total != want {
		t.Errorf("totals = report %+v, package %+v, file %+v; want %+v",
			got.Total, got.Packages[0].Total, got.Packages[0].Files[0].Total, want)
	}
}

func TestOnto_TotalsUnchangedWhenNothingImproves(t *testing.T) {
	base := reportWithFile(report.NewCoverageStats(12, 20),
		report.FuncReport{Name: "Cold", TotalStatements: 8, CoveredStatements: 2, CoveragePercent: 25},
	)
	other := reportWithFile(report.NewCoverageStats(0, 0),
		report.FuncReport{Name: "Cold", CoveragePercent: 10},
	)

	if got, want := Onto(base, other).Total, report.NewCoverageStats(12, 20); got != want {
		t.Errorf("total = %+v, want base's %+v", got, want)
	}
}

func TestOnto_StatementCountsComeFromBase(t *testing.T) {
	// Foo had 10 statements in the old build and has 12 now. The old build's
	// percentage wins, but its statement count describes code that changed.
	base := reportWithFile(report.NewCoverageStats(3, 12),
		report.FuncReport{Name: "Foo", TotalStatements: 12, CoveredStatements: 3, CoveragePercent: 25},
	)
	old := reportWithFile(report.NewCoverageStats(10, 10),
		report.FuncReport{Name: "Foo", TotalStatements: 10, CoveredStatements: 10, CoveragePercent: 100},
	)

	got := Onto(base, old)

	foo := findFunc(got, "Foo")
	if foo.CoveragePercent != 100 || foo.TotalStatements != 12 || foo.CoveredStatements != 12 {
		t.Errorf("Foo = %.0f%% (%d/%d), want 100%% (12/12)", foo.CoveragePercent, foo.CoveredStatements, foo.TotalStatements)
	}
	if want := report.NewCoverageStats(12, 12); got.Total != want {
		t.Errorf("total = %+v, want %+v", got.Total, want)
	}
}

func TestOnto_TotalsFallBackToFunctionsWhenBaseHasNone(t *testing.T) {
	// A base without usable totals (hand-written, or totals smaller than its
	// functions) has no remainder to carry: totals are the sum of functions.
	base := reportWithFile(report.CoverageStats{},
		report.FuncReport{Name: "A", TotalStatements: 4, CoveredStatements: 1, CoveragePercent: 25},
		report.FuncReport{Name: "B", TotalStatements: 6, CoveredStatements: 6, CoveragePercent: 100},
	)
	other := reportWithFile(report.CoverageStats{},
		report.FuncReport{Name: "A", CoveragePercent: 50},
	)

	if got, want := Onto(base, other).Total, report.NewCoverageStats(8, 10); got != want {
		t.Errorf("total = %+v, want %+v", got, want)
	}
}

func TestCoverName(t *testing.T) {
	tests := map[string]string{
		"Foo":                      "Foo",
		"(*Server).Handle":         "(*Server).Handle",
		"(Server).Handle":          "(Server).Handle",
		"(*List[T]).Push":          "Push",
		"(Pair[K, V]).Swap":        "Swap",
		"(*Map[K, []V]).Get":       "Get",
		"(*report.Report).Clone":   "(*report.Report).Clone",
		"(*List[map[K]V]).Push":    "Push",
		"(*Weird).Method).Strange": "(*Weird).Method).Strange",
	}
	for in, want := range tests {
		if got := coverName(in); got != want {
			t.Errorf("coverName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestOnto_GenericMethodMatchesCovdataName checks that an older build's
// covdata entry for a generic method, which cmd/cover names by the bare
// method name, is matched to the astmap name.
func TestOnto_GenericMethodMatchesCovdataName(t *testing.T) {
	stats := report.NewCoverageStats(0, 10)
	base := reportWithFile(stats, report.FuncReport{
		Name: "(*List[T]).Push", Line: 5, TotalStatements: 10, CoveragePercent: 0,
		UnreachedBlocks: []report.UnreachedBlock{{StartLine: 6, EndLine: 8, NumStatements: 10}},
	})
	older := reportWithFile(report.CoverageStats{}, report.FuncReport{Name: "Push", CoveragePercent: 100})

	got := findFunc(Onto(base, older), "(*List[T]).Push")
	if got == nil || got.CoveragePercent != 100 || got.CoveredStatements != 10 {
		t.Fatalf("got %+v, want 10/10 at 100%% from the older build", got)
	}
	if len(got.LatestUnreachedBlocks) != 1 {
		t.Errorf("LatestUnreachedBlocks = %v, want the base's block", got.LatestUnreachedBlocks)
	}
}

// TestOnto_DuplicateNamesKeepBase checks that several functions sharing a
// name in one file (multiple init funcs) keep their own values instead of
// all taking the best one's.
func TestOnto_DuplicateNamesKeepBase(t *testing.T) {
	initA := report.FuncReport{Name: "init", Line: 3, TotalStatements: 2, CoveredStatements: 0, CoveragePercent: 0,
		UnreachedBlocks: []report.UnreachedBlock{{StartLine: 3, EndLine: 4, NumStatements: 2}}}
	initB := report.FuncReport{Name: "init", Line: 10, TotalStatements: 10, CoveredStatements: 10, CoveragePercent: 100}
	base := reportWithFile(report.NewCoverageStats(10, 12), initA, initB)
	older := reportWithFile(report.CoverageStats{}, report.FuncReport{Name: "init", CoveragePercent: 100})

	merged := Onto(base, older)
	fns := merged.Packages[0].Files[0].Functions
	if fns[0].TotalStatements != 2 || fns[0].CoveredStatements != 0 || len(fns[0].UnreachedBlocks) != 1 {
		t.Errorf("first init = %+v, want its own 0/2 and unreached block", fns[0])
	}
	if fns[1].TotalStatements != 10 || fns[1].CoveredStatements != 10 {
		t.Errorf("second init = %+v, want its own 10/10", fns[1])
	}
	if merged.Total.TotalStatements != 12 || merged.Total.CoveredStatements != 10 {
		t.Errorf("total = %+v, want 10/12", merged.Total)
	}
}

// TestOnto_RemergeKeepsLatestBlocks checks that merging onto a report that
// is itself a merge result keeps the current-source blocks.
func TestOnto_RemergeKeepsLatestBlocks(t *testing.T) {
	oldBlocks := []report.UnreachedBlock{{StartLine: 100, EndLine: 101, NumStatements: 1}}
	curBlocks := []report.UnreachedBlock{{StartLine: 6, EndLine: 7, NumStatements: 2}}
	// A merge result where an older build won: UnreachedBlocks are from old
	// source, LatestUnreachedBlocks from the current one.
	merged1 := reportWithFile(report.NewCoverageStats(8, 10), report.FuncReport{
		Name: "Foo", Line: 5, TotalStatements: 10, CoveredStatements: 8, CoveragePercent: 80,
		UnreachedBlocks: oldBlocks, LatestUnreachedBlocks: curBlocks,
	})

	t.Run("base wins", func(t *testing.T) {
		other := reportWithFile(report.CoverageStats{}, report.FuncReport{Name: "Foo", CoveragePercent: 50})
		got := findFunc(Onto(merged1, other), "Foo")
		if len(got.LatestUnreachedBlocks) != 1 || got.LatestUnreachedBlocks[0].StartLine != 6 {
			t.Errorf("LatestUnreachedBlocks = %v, want the current-source block carried over", got.LatestUnreachedBlocks)
		}
	})
	t.Run("other wins", func(t *testing.T) {
		other := reportWithFile(report.CoverageStats{}, report.FuncReport{Name: "Foo", CoveragePercent: 90})
		got := findFunc(Onto(merged1, other), "Foo")
		if len(got.LatestUnreachedBlocks) != 1 || got.LatestUnreachedBlocks[0].StartLine != 6 {
			t.Errorf("LatestUnreachedBlocks = %v, want the current-source block, not old-source lines", got.LatestUnreachedBlocks)
		}
	})
}

// TestOnto_GenericMethodsMatchExactlyBetweenReports checks that reports
// carrying full names still merge generic methods one to one, even when two
// types in a file share a method name, and that a bare covdata name which
// could mean either of them is not applied to both.
func TestOnto_GenericMethodsMatchExactlyBetweenReports(t *testing.T) {
	fn := func(name string, covered int) report.FuncReport {
		return report.FuncReport{Name: name, TotalStatements: 10, CoveredStatements: covered,
			CoveragePercent: report.ComputePercent(covered, 10)}
	}
	base := reportWithFile(report.NewCoverageStats(0, 20), fn("(*List[T]).Len", 0), fn("(*Map[K, V]).Len", 0))
	older := reportWithFile(report.NewCoverageStats(10, 20), fn("(*List[T]).Len", 10), fn("(*Map[K, V]).Len", 0))
	covdata := reportWithFile(report.CoverageStats{}, report.FuncReport{Name: "Len", CoveragePercent: 100})

	merged := Onto(base, older, covdata)
	if got := findFunc(merged, "(*List[T]).Len"); got.CoveredStatements != 10 {
		t.Errorf("(*List[T]).Len = %+v, want 10/10 from the older report", got)
	}
	if got := findFunc(merged, "(*Map[K, V]).Len"); got.CoveredStatements != 0 {
		t.Errorf("(*Map[K, V]).Len = %+v, want 0/10: the bare name Len is ambiguous", got)
	}
}

// TestOnto_GenericMethodNotMatchedWhenPlainFuncSharesName checks that a bare
// covdata name is not applied to a generic method when the file also has a
// plain function of that name.
func TestOnto_GenericMethodNotMatchedWhenPlainFuncSharesName(t *testing.T) {
	base := reportWithFile(report.NewCoverageStats(0, 20),
		report.FuncReport{Name: "(*List[T]).Push", TotalStatements: 10},
		report.FuncReport{Name: "Push", TotalStatements: 10},
	)
	// cmd/cover names both "Push"; which line is which cannot be told.
	covdata := reportWithFile(report.CoverageStats{},
		report.FuncReport{Name: "Push", CoveragePercent: 100},
		report.FuncReport{Name: "Push", CoveragePercent: 0},
	)

	merged := Onto(base, covdata)
	for _, name := range []string{"(*List[T]).Push", "Push"} {
		if got := findFunc(merged, name); got.CoveredStatements != 0 || got.CoveragePercent != 0 {
			t.Errorf("%s = %+v, want base's 0/10 kept", name, got)
		}
	}
}

func calls(n int) *int { return &n }

// TestOnto_CallsAddUp checks that call counts are summed across reports,
// unlike coverage, where the maximum wins.
func TestOnto_CallsAddUp(t *testing.T) {
	fn := func(name string, c *int) report.FuncReport {
		return report.FuncReport{Name: name, TotalStatements: 10, Calls: c}
	}
	base := reportWithFile(report.NewCoverageStats(0, 40),
		fn("Both", calls(3)), fn("BaseOnly", calls(5)), fn("OtherOnly", nil), fn("Neither", nil),
		fn("(*List[T]).Push", calls(1)))
	other := reportWithFile(report.CoverageStats{},
		fn("Both", calls(4)), fn("BaseOnly", nil), fn("OtherOnly", calls(0)),
		fn("Push", calls(6)), // covdata's name for the generic method
		fn("Gone", calls(99)))
	third := reportWithFile(report.CoverageStats{}, fn("Both", calls(10)))

	merged := Onto(base, other, third)
	want := map[string]*int{
		"Both":            calls(17),
		"BaseOnly":        calls(5),
		"OtherOnly":       calls(0),
		"Neither":         nil,
		"(*List[T]).Push": calls(7),
	}
	for name, w := range want {
		got := findFunc(merged, name).Calls
		switch {
		case w == nil && got != nil:
			t.Errorf("%s: calls = %d, want none", name, *got)
		case w != nil && (got == nil || *got != *w):
			t.Errorf("%s: calls = %v, want %d", name, got, *w)
		}
	}
	if findFunc(merged, "Gone") != nil {
		t.Error("a function only in an older report must stay out")
	}
}

// TestOnto_CallsOfAmbiguousNamesStayWithBase checks that functions sharing a
// name keep their own counts instead of each getting the others' as well.
func TestOnto_CallsOfAmbiguousNamesStayWithBase(t *testing.T) {
	base := reportWithFile(report.NewCoverageStats(0, 4),
		report.FuncReport{Name: "init", Line: 3, TotalStatements: 2, Calls: calls(1)},
		report.FuncReport{Name: "init", Line: 9, TotalStatements: 2, Calls: calls(2)})
	other := reportWithFile(report.CoverageStats{}, report.FuncReport{Name: "init", Calls: calls(50)})

	fns := Onto(base, other).Packages[0].Files[0].Functions
	if *fns[0].Calls != 1 || *fns[1].Calls != 2 {
		t.Errorf("calls = %d, %d; want base's 1, 2", *fns[0].Calls, *fns[1].Calls)
	}
}
