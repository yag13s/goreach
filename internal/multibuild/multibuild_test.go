package multibuild

import (
	"slices"
	"testing"

	"github.com/yag13s/goreach/internal/covparse"
	"github.com/yag13s/goreach/internal/report"
)

func TestReportFromFuncCoverage(t *testing.T) {
	r := reportFromFuncCoverage([]covparse.FuncCoverage{
		{FileName: "example.com/app/sub/z.go", FuncName: "Z", CoveragePercent: 10},
		{FileName: "example.com/app/b.go", FuncName: "B2", CoveragePercent: 20},
		{FileName: "example.com/app/sub.go", FuncName: "S", CoveragePercent: 30},
		{FileName: "example.com/app/b.go", FuncName: "(*T).B1", CoveragePercent: 40},
	})

	if r.Mode != "covdata-func" || r.Version != 1 {
		t.Errorf("mode/version = %q/%d, want covdata-func/1", r.Mode, r.Version)
	}

	// "app/sub.go" sorts between "app/b.go" and "app/sub/z.go" as a plain
	// string, but must still land in package "app".
	type file struct {
		pkg, name string
		funcs     []string
	}
	want := []file{
		{"example.com/app", "example.com/app/b.go", []string{"B2", "(*T).B1"}},
		{"example.com/app", "example.com/app/sub.go", []string{"S"}},
		{"example.com/app/sub", "example.com/app/sub/z.go", []string{"Z"}},
	}
	var got []file
	for _, pkg := range r.Packages {
		for _, f := range pkg.Files {
			var names []string
			for _, fn := range f.Functions {
				names = append(names, fn.Name)
				if fn.TotalStatements != 0 || fn.CoveredStatements != 0 {
					t.Errorf("%s: statement counts should be zero, got %d/%d", fn.Name, fn.CoveredStatements, fn.TotalStatements)
				}
			}
			got = append(got, file{pkg.ImportPath, f.FileName, names})
		}
	}
	if len(r.Packages) != 2 {
		t.Errorf("got %d packages, want 2", len(r.Packages))
	}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].pkg != want[i].pkg || got[i].name != want[i].name || !slices.Equal(got[i].funcs, want[i].funcs) {
			t.Errorf("file %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReportFromFuncCoverage_Empty(t *testing.T) {
	r := reportFromFuncCoverage(nil)
	if len(r.Packages) != 0 {
		t.Errorf("got %d packages, want 0", len(r.Packages))
	}
}

// newestReport is what analysis.Run would produce for the current build:
// real statement counts and source-accurate unreached blocks.
func newestReport() *report.Report {
	return &report.Report{
		Version: 1,
		Mode:    "atomic",
		Packages: []report.PackageReport{{
			ImportPath: "example.com/app",
			Files: []report.FileReport{{
				FileName: "example.com/app/a.go",
				Functions: []report.FuncReport{
					{
						Name: "Hot", Line: 10,
						TotalStatements: 4, CoveredStatements: 4, CoveragePercent: 100,
					},
					{
						Name: "Cold", Line: 20,
						TotalStatements: 4, CoveredStatements: 1, CoveragePercent: 25,
						UnreachedBlocks: []report.UnreachedBlock{{StartLine: 22, EndLine: 24, NumStatements: 3}},
					},
					{
						Name: "New", Line: 30,
						TotalStatements: 2, CoveredStatements: 0, CoveragePercent: 0,
						UnreachedBlocks: []report.UnreachedBlock{{StartLine: 31, EndLine: 32, NumStatements: 2}},
					},
				},
			}},
		}},
	}
}

func TestCombine(t *testing.T) {
	older := [][]covparse.FuncCoverage{
		{
			{FileName: "example.com/app/a.go", FuncName: "Hot", CoveragePercent: 50},
			{FileName: "example.com/app/a.go", FuncName: "Cold", CoveragePercent: 75},
			{FileName: "example.com/app/a.go", FuncName: "Deleted", CoveragePercent: 100},
		},
		{
			{FileName: "example.com/app/a.go", FuncName: "Cold", CoveragePercent: 50},
		},
	}

	got := combine(newestReport(), older)

	if got.Mode != "merged" {
		t.Errorf("mode = %q, want merged", got.Mode)
	}
	funcs := got.Packages[0].Files[0].Functions
	if len(funcs) != 3 {
		t.Fatalf("got %d functions, want 3 (functions only in older builds are dropped)", len(funcs))
	}

	// Newest build already has the best coverage: kept as is.
	hot := funcs[0]
	if hot.CoveragePercent != 100 || hot.CoveredStatements != 4 || hot.LatestUnreachedBlocks != nil {
		t.Errorf("Hot = %+v", hot)
	}

	// An older build wins: its percentage is applied to the newest build's
	// statement count, and the newest build's blocks are kept for the viewer.
	cold := funcs[1]
	if cold.CoveragePercent != 75 || cold.TotalStatements != 4 || cold.CoveredStatements != 3 {
		t.Errorf("Cold coverage = %.0f%% (%d/%d), want 75%% (3/4)", cold.CoveragePercent, cold.CoveredStatements, cold.TotalStatements)
	}
	if cold.Line != 20 {
		t.Errorf("Cold line = %d, want 20 (newest source)", cold.Line)
	}
	if len(cold.UnreachedBlocks) != 0 {
		t.Errorf("Cold unreached blocks = %+v, want none (covdata func has no block data)", cold.UnreachedBlocks)
	}
	if len(cold.LatestUnreachedBlocks) != 1 || cold.LatestUnreachedBlocks[0].StartLine != 22 {
		t.Errorf("Cold latest unreached blocks = %+v", cold.LatestUnreachedBlocks)
	}

	// Only exists in the newest build.
	if fn := funcs[2]; fn.Name != "New" || fn.CoveragePercent != 0 || len(fn.UnreachedBlocks) != 1 {
		t.Errorf("New = %+v", fn)
	}

	if got.Total.TotalStatements != 10 || got.Total.CoveredStatements != 7 {
		t.Errorf("total = %d/%d, want 7/10", got.Total.CoveredStatements, got.Total.TotalStatements)
	}
}
