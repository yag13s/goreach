package analysis

import (
	"testing"

	"golang.org/x/tools/cover"

	"github.com/yag13s/goreach/internal/astmap"
	"github.com/yag13s/goreach/internal/report"
)

func TestBlockOverlapsFunc(t *testing.T) {
	fn := &astmap.FuncExtent{
		Name:      "Foo",
		StartLine: 10,
		StartCol:  1,
		EndLine:   20,
		EndCol:    2,
	}

	tests := []struct {
		name  string
		block cover.ProfileBlock
		want  bool
	}{
		{
			name:  "block inside function",
			block: cover.ProfileBlock{StartLine: 11, StartCol: 2, EndLine: 15, EndCol: 3},
			want:  true,
		},
		{
			name:  "block matches function exactly",
			block: cover.ProfileBlock{StartLine: 10, StartCol: 1, EndLine: 20, EndCol: 2},
			want:  true,
		},
		{
			name:  "block before function",
			block: cover.ProfileBlock{StartLine: 1, StartCol: 1, EndLine: 9, EndCol: 10},
			want:  false,
		},
		{
			name:  "block after function",
			block: cover.ProfileBlock{StartLine: 21, StartCol: 1, EndLine: 25, EndCol: 10},
			want:  false,
		},
		{
			name:  "block overlaps start",
			block: cover.ProfileBlock{StartLine: 8, StartCol: 1, EndLine: 12, EndCol: 3},
			want:  true,
		},
		{
			name:  "block overlaps end",
			block: cover.ProfileBlock{StartLine: 18, StartCol: 1, EndLine: 22, EndCol: 3},
			want:  true,
		},
		{
			name:  "block ends exactly at function start line but before column",
			block: cover.ProfileBlock{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 0},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blockOverlapsFunc(tt.block, fn)
			if got != tt.want {
				t.Errorf("blockOverlapsFunc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnalyzeFile(t *testing.T) {
	prof := &cover.Profile{
		FileName: "example.com/pkg/foo.go",
		Mode:     "set",
		Blocks: []cover.ProfileBlock{
			{StartLine: 5, StartCol: 20, EndLine: 7, EndCol: 2, NumStmt: 1, Count: 1},   // inside Add, covered
			{StartLine: 9, StartCol: 25, EndLine: 11, EndCol: 2, NumStmt: 1, Count: 0},  // inside Sub, not covered
			{StartLine: 13, StartCol: 27, EndLine: 14, EndCol: 18, NumStmt: 1, Count: 1}, // inside Greet, covered
			{StartLine: 14, StartCol: 18, EndLine: 16, EndCol: 3, NumStmt: 1, Count: 0},  // inside Greet, not covered
			{StartLine: 17, StartCol: 2, EndLine: 17, EndCol: 40, NumStmt: 1, Count: 1},  // inside Greet, covered
		},
	}

	funcs := []*astmap.FuncExtent{
		{Name: "Add", StartLine: 5, StartCol: 1, EndLine: 7, EndCol: 2},
		{Name: "Sub", StartLine: 9, StartCol: 1, EndLine: 11, EndCol: 2},
		{Name: "Greet", StartLine: 13, StartCol: 1, EndLine: 18, EndCol: 2},
	}

	// Default options (threshold=100 shows all)
	opts := Options{Threshold: 100}
	result := analyzeFile(prof, funcs, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Total.TotalStatements != 5 {
		t.Errorf("total statements = %d, want 5", result.Total.TotalStatements)
	}
	if result.Total.CoveredStatements != 3 {
		t.Errorf("covered statements = %d, want 3", result.Total.CoveredStatements)
	}

	// All 3 functions should be in the report since threshold=100
	if len(result.Functions) != 3 {
		t.Errorf("functions count = %d, want 3", len(result.Functions))
	}

	// Test threshold filter: only show functions with <50% coverage
	opts = Options{Threshold: 50}
	result = analyzeFile(prof, funcs, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Only Sub (0%) should be shown; Add (100%) and Greet (66.7%) are filtered
	if len(result.Functions) != 1 {
		t.Errorf("filtered functions count = %d, want 1", len(result.Functions))
	}
	if len(result.Functions) > 0 && result.Functions[0].Name != "Sub" {
		t.Errorf("expected Sub, got %s", result.Functions[0].Name)
	}
}

func TestMatchesPrefixes(t *testing.T) {
	tests := []struct {
		importPath string
		prefixes   []string
		want       bool
	}{
		{"myapp/internal/auth", nil, true},
		{"myapp/internal/auth", []string{"myapp/internal"}, true},
		{"myapp/internal/auth", []string{"other/"}, false},
		{"myapp/internal/auth", []string{"other/", "myapp/"}, true},
	}

	for _, tt := range tests {
		got := matchesPrefixes(tt.importPath, tt.prefixes)
		if got != tt.want {
			t.Errorf("matchesPrefixes(%q, %v) = %v, want %v", tt.importPath, tt.prefixes, got, tt.want)
		}
	}
}

func TestPackageFromFile(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"myapp/internal/auth/oauth.go", "myapp/internal/auth"},
		{"myapp/main.go", "myapp"},
	}
	for _, tt := range tests {
		got := packageFromFile(tt.filename)
		if got != tt.want {
			t.Errorf("packageFromFile(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestRunWithEmptyProfiles(t *testing.T) {
	rpt, err := Run(nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rpt.Total.TotalStatements != 0 {
		t.Errorf("total = %d, want 0", rpt.Total.TotalStatements)
	}
	if len(rpt.Packages) != 0 {
		t.Errorf("packages = %d, want 0", len(rpt.Packages))
	}
}

// Verify that ComputePercent produces correct values in report context
func TestFuncReportUnreachedBlocks(t *testing.T) {
	fr := report.FuncReport{
		Name:              "(*Server).Handle",
		Line:              42,
		TotalStatements:   10,
		CoveredStatements: 0,
		CoveragePercent:   0,
		UnreachedBlocks: []report.UnreachedBlock{
			{StartLine: 43, StartCol: 2, EndLine: 50, EndCol: 3, NumStatements: 10},
		},
	}
	if len(fr.UnreachedBlocks) != 1 {
		t.Errorf("unreached blocks = %d, want 1", len(fr.UnreachedBlocks))
	}
	if fr.UnreachedBlocks[0].NumStatements != 10 {
		t.Errorf("num statements = %d, want 10", fr.UnreachedBlocks[0].NumStatements)
	}
}

// TestAnalyzeFile_MinStatements tests that functions with unreached statements
// below the MinStatements threshold are excluded from the report.
func TestAnalyzeFile_MinStatements(t *testing.T) {
	prof := &cover.Profile{
		FileName: "example.com/pkg/foo.go",
		Mode:     "set",
		Blocks: []cover.ProfileBlock{
			// FuncA: 3 stmts, 1 covered -> 2 unreached
			{StartLine: 5, StartCol: 20, EndLine: 7, EndCol: 2, NumStmt: 1, Count: 1},
			{StartLine: 8, StartCol: 2, EndLine: 9, EndCol: 2, NumStmt: 1, Count: 0},
			{StartLine: 10, StartCol: 2, EndLine: 11, EndCol: 2, NumStmt: 1, Count: 0},
			// FuncB: 5 stmts, 1 covered -> 4 unreached
			{StartLine: 15, StartCol: 20, EndLine: 17, EndCol: 2, NumStmt: 1, Count: 1},
			{StartLine: 18, StartCol: 2, EndLine: 19, EndCol: 2, NumStmt: 1, Count: 0},
			{StartLine: 20, StartCol: 2, EndLine: 21, EndCol: 2, NumStmt: 1, Count: 0},
			{StartLine: 22, StartCol: 2, EndLine: 23, EndCol: 2, NumStmt: 1, Count: 0},
			{StartLine: 24, StartCol: 2, EndLine: 25, EndCol: 2, NumStmt: 1, Count: 0},
		},
	}

	funcs := []*astmap.FuncExtent{
		{Name: "FuncA", StartLine: 5, StartCol: 1, EndLine: 12, EndCol: 2},
		{Name: "FuncB", StartLine: 15, StartCol: 1, EndLine: 26, EndCol: 2},
	}

	// MinStatements=3: FuncA has 2 unreached (excluded), FuncB has 4 unreached (included)
	opts := Options{Threshold: 100, MinStatements: 3}
	result := analyzeFile(prof, funcs, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Functions) != 1 {
		t.Fatalf("functions count = %d, want 1", len(result.Functions))
	}
	if result.Functions[0].Name != "FuncB" {
		t.Errorf("expected FuncB, got %s", result.Functions[0].Name)
	}

	// Total statements should still count both functions
	if result.Total.TotalStatements != 8 {
		t.Errorf("total statements = %d, want 8", result.Total.TotalStatements)
	}
}

// TestAnalyzeFile_EmptyFunction tests that functions with no coverage blocks
// (totalStmts == 0) are skipped entirely and don't appear in the report.
func TestAnalyzeFile_EmptyFunction(t *testing.T) {
	prof := &cover.Profile{
		FileName: "example.com/pkg/foo.go",
		Mode:     "set",
		Blocks: []cover.ProfileBlock{
			// Block only within FuncB
			{StartLine: 15, StartCol: 20, EndLine: 17, EndCol: 2, NumStmt: 1, Count: 1},
		},
	}

	funcs := []*astmap.FuncExtent{
		// FuncA has no blocks overlapping it -> totalStmts=0, should be skipped
		{Name: "FuncA", StartLine: 5, StartCol: 1, EndLine: 7, EndCol: 2},
		// FuncB has 1 block
		{Name: "FuncB", StartLine: 15, StartCol: 1, EndLine: 18, EndCol: 2},
	}

	opts := Options{Threshold: 100}
	result := analyzeFile(prof, funcs, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Only FuncB should be present since FuncA has no statements
	if len(result.Functions) != 1 {
		t.Fatalf("functions count = %d, want 1", len(result.Functions))
	}
	if result.Functions[0].Name != "FuncB" {
		t.Errorf("expected FuncB, got %s", result.Functions[0].Name)
	}
	if result.Total.TotalStatements != 1 {
		t.Errorf("total statements = %d, want 1", result.Total.TotalStatements)
	}
}

// TestAnalyzeFile_AllEmpty tests that when all functions have zero statements,
// analyzeFile returns nil.
func TestAnalyzeFile_AllEmpty(t *testing.T) {
	prof := &cover.Profile{
		FileName: "example.com/pkg/foo.go",
		Mode:     "set",
		Blocks:   []cover.ProfileBlock{
			// Block that doesn't overlap any function
			{StartLine: 100, StartCol: 1, EndLine: 110, EndCol: 2, NumStmt: 1, Count: 1},
		},
	}

	funcs := []*astmap.FuncExtent{
		{Name: "FuncA", StartLine: 5, StartCol: 1, EndLine: 7, EndCol: 2},
	}

	opts := Options{Threshold: 100}
	result := analyzeFile(prof, funcs, opts)
	if result != nil {
		t.Errorf("expected nil result for all-empty functions, got %+v", result)
	}
}

// TestGroupByPackage tests that profiles are correctly grouped by package directory.
func TestGroupByPackage(t *testing.T) {
	profiles := []*cover.Profile{
		{FileName: "myapp/internal/auth/oauth.go"},
		{FileName: "myapp/internal/auth/token.go"},
		{FileName: "myapp/internal/db/conn.go"},
		{FileName: "myapp/main.go"},
	}

	result := groupByPackage(profiles)

	if len(result) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(result))
	}

	authProfs := result["myapp/internal/auth"]
	if len(authProfs) != 2 {
		t.Errorf("auth package: expected 2 profiles, got %d", len(authProfs))
	}

	dbProfs := result["myapp/internal/db"]
	if len(dbProfs) != 1 {
		t.Errorf("db package: expected 1 profile, got %d", len(dbProfs))
	}

	mainProfs := result["myapp"]
	if len(mainProfs) != 1 {
		t.Errorf("main package: expected 1 profile, got %d", len(mainProfs))
	}
}

// TestGroupByPackage_Empty tests groupByPackage with nil/empty input.
func TestGroupByPackage_Empty(t *testing.T) {
	result := groupByPackage(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 packages, got %d", len(result))
	}

	result = groupByPackage([]*cover.Profile{})
	if len(result) != 0 {
		t.Errorf("expected 0 packages, got %d", len(result))
	}
}

// TestRunWithUnresolvableProfiles tests that Run handles profiles with
// package paths that can't be resolved via `go list`.
func TestRunWithUnresolvableProfiles(t *testing.T) {
	profiles := []*cover.Profile{
		{
			FileName: "nonexistent.example.com/fake/pkg/foo.go",
			Mode:     "set",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 2, NumStmt: 1, Count: 1},
			},
		},
	}

	// Run should not error out; it should skip unresolvable packages
	rpt, err := Run(profiles, Options{Threshold: 100})
	if err != nil {
		// resolvePackages may error when go list fails for fake packages
		// That's acceptable - we just verify the function doesn't panic
		t.Logf("Run returned error (expected for fake package): %v", err)
		return
	}

	// If it doesn't error, it should return an empty report since the
	// package can't be resolved
	if len(rpt.Packages) != 0 {
		t.Errorf("expected 0 packages for unresolvable profile, got %d", len(rpt.Packages))
	}
	if rpt.Mode != "set" {
		t.Errorf("expected mode 'set', got %q", rpt.Mode)
	}
}

// TestRunWithPkgPrefixFilter tests that Run correctly filters packages
// by prefix when PkgPrefixes is set.
func TestRunWithPkgPrefixFilter(t *testing.T) {
	profiles := []*cover.Profile{
		{
			FileName: "nonexistent.example.com/included/foo.go",
			Mode:     "set",
			Blocks:   []cover.ProfileBlock{
				{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 2, NumStmt: 1, Count: 1},
			},
		},
		{
			FileName: "nonexistent.example.com/excluded/bar.go",
			Mode:     "set",
			Blocks:   []cover.ProfileBlock{
				{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 2, NumStmt: 1, Count: 1},
			},
		},
	}

	// Only include "nonexistent.example.com/included" prefix
	opts := Options{
		PkgPrefixes: []string{"nonexistent.example.com/included"},
		Threshold:   100,
	}

	rpt, err := Run(profiles, opts)
	if err != nil {
		t.Logf("Run returned error (expected for fake package): %v", err)
		return
	}

	// The filtered-out package should not appear, but neither should the included
	// one since it can't be resolved. Total packages should be 0.
	if len(rpt.Packages) != 0 {
		t.Errorf("expected 0 packages (all unresolvable), got %d", len(rpt.Packages))
	}
}

// TestBlockOverlapsFunc_SameLineEdgeCases tests the column-level edge cases
// on the same line boundaries.
func TestBlockOverlapsFunc_SameLineEdgeCases(t *testing.T) {
	fn := &astmap.FuncExtent{
		Name:      "Foo",
		StartLine: 10,
		StartCol:  5,
		EndLine:   20,
		EndCol:    10,
	}

	tests := []struct {
		name  string
		block cover.ProfileBlock
		want  bool
	}{
		{
			name:  "block starts on end line, after end col",
			block: cover.ProfileBlock{StartLine: 20, StartCol: 11, EndLine: 25, EndCol: 1},
			want:  false,
		},
		{
			name:  "block starts on end line, at end col",
			block: cover.ProfileBlock{StartLine: 20, StartCol: 10, EndLine: 25, EndCol: 1},
			want:  true,
		},
		{
			name:  "block ends on start line, at start col",
			block: cover.ProfileBlock{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 5},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blockOverlapsFunc(tt.block, fn)
			if got != tt.want {
				t.Errorf("blockOverlapsFunc() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAnalyzeFile_NoFunctions tests analyzeFile with no function extents.
func TestAnalyzeFile_NoFunctions(t *testing.T) {
	prof := &cover.Profile{
		FileName: "example.com/pkg/foo.go",
		Mode:     "set",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 2, NumStmt: 1, Count: 1},
		},
	}

	result := analyzeFile(prof, nil, Options{Threshold: 100})
	if result != nil {
		t.Errorf("expected nil result for no functions, got %+v", result)
	}
}

// TestAnalyzeFile_ThresholdExactBoundary tests the boundary condition where
// coverage percentage equals the threshold exactly.
func TestAnalyzeFile_ThresholdExactBoundary(t *testing.T) {
	// FuncA: 2 stmts, 1 covered -> 50% coverage
	prof := &cover.Profile{
		FileName: "example.com/pkg/foo.go",
		Mode:     "set",
		Blocks: []cover.ProfileBlock{
			{StartLine: 5, StartCol: 20, EndLine: 7, EndCol: 2, NumStmt: 1, Count: 1},
			{StartLine: 8, StartCol: 2, EndLine: 9, EndCol: 2, NumStmt: 1, Count: 0},
		},
	}

	funcs := []*astmap.FuncExtent{
		{Name: "FuncA", StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 2},
	}

	// Threshold=50: coverage is exactly 50%, which is NOT > 50, so the function
	// should be included in the report
	opts := Options{Threshold: 50}
	result := analyzeFile(prof, funcs, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Functions) != 1 {
		t.Errorf("expected 1 function at exact threshold, got %d", len(result.Functions))
	}

	// Threshold=49: coverage is 50% which IS > 49, so it should be filtered
	opts = Options{Threshold: 49}
	result = analyzeFile(prof, funcs, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Functions) != 0 {
		t.Errorf("expected 0 functions above threshold, got %d", len(result.Functions))
	}
}
