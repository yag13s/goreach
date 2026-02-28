package covparse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeCovdataFuncName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"FuncName", "FuncName"},
		{"*Type.Method", "(*Type).Method"},
		{"Type.Method", "(Type).Method"},
		{"*Type[go.shape.int].Method", "(*Type[go.shape.int]).Method"},
		{"Type[go.shape.int].Method", "(Type[go.shape.int]).Method"},
	}
	for _, tt := range tests {
		got := NormalizeCovdataFuncName(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeCovdataFuncName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseCovdataFuncOutput(t *testing.T) {
	output := `github.com/user/pkg/handler.go:10:	HandleRequest	75.0%
github.com/user/pkg/handler.go:25:	*Server.Start	100.0%
github.com/user/pkg/handler.go:40:	Server.Stop	0.0%
github.com/user/pkg/util.go:5:		Helper	50.0%
total	(statements)	62.5%
`
	funcs := parseCovdataFuncOutput(output)

	if len(funcs) != 4 {
		t.Fatalf("expected 4 functions, got %d", len(funcs))
	}

	// Check first function
	if funcs[0].FileName != "github.com/user/pkg/handler.go" {
		t.Errorf("funcs[0].FileName = %q", funcs[0].FileName)
	}
	if funcs[0].FuncName != "HandleRequest" {
		t.Errorf("funcs[0].FuncName = %q", funcs[0].FuncName)
	}
	if funcs[0].CoveragePercent != 75.0 {
		t.Errorf("funcs[0].CoveragePercent = %v", funcs[0].CoveragePercent)
	}

	// Check pointer receiver
	if funcs[1].FuncName != "(*Server).Start" {
		t.Errorf("funcs[1].FuncName = %q, want (*Server).Start", funcs[1].FuncName)
	}
	if funcs[1].CoveragePercent != 100.0 {
		t.Errorf("funcs[1].CoveragePercent = %v", funcs[1].CoveragePercent)
	}

	// Check value receiver
	if funcs[2].FuncName != "(Server).Stop" {
		t.Errorf("funcs[2].FuncName = %q, want (Server).Stop", funcs[2].FuncName)
	}

	// Check util.go file
	if funcs[3].FileName != "github.com/user/pkg/util.go" {
		t.Errorf("funcs[3].FileName = %q", funcs[3].FileName)
	}
	if funcs[3].FuncName != "Helper" {
		t.Errorf("funcs[3].FuncName = %q", funcs[3].FuncName)
	}
}

func TestParseCovdataFuncOutput_Empty(t *testing.T) {
	funcs := parseCovdataFuncOutput("")
	if len(funcs) != 0 {
		t.Errorf("expected 0 functions for empty output, got %d", len(funcs))
	}
}

func TestParseCovdataFuncOutput_TotalOnly(t *testing.T) {
	funcs := parseCovdataFuncOutput("total	(statements)	50.0%\n")
	if len(funcs) != 0 {
		t.Errorf("expected 0 functions for total-only output, got %d", len(funcs))
	}
}

func TestNewestCounterTime(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "d1")
	dir2 := filepath.Join(root, "d2")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Write covcounters files with different timestamps
	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)

	f1 := filepath.Join(dir1, "covcounters.abc")
	if err := os.WriteFile(f1, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f1, older, older); err != nil {
		t.Fatal(err)
	}

	f2 := filepath.Join(dir2, "covcounters.def")
	if err := os.WriteFile(f2, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f2, newer, newer); err != nil {
		t.Fatal(err)
	}

	ts, err := newestCounterTime([]string{dir1, dir2})
	if err != nil {
		t.Fatal(err)
	}

	// Should match the newer file's time (within 1 second tolerance)
	diff := ts.Sub(newer)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("newest time %v differs from expected %v by %v", ts, newer, diff)
	}
}

func TestNewestCounterTime_NoCovCounters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "covmeta.abc"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts, err := newestCounterTime([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if !ts.IsZero() {
		t.Errorf("expected zero time for dir with no covcounters, got %v", ts)
	}
}

func TestParseDirRecursiveGrouped_Ordering(t *testing.T) {
	root := t.TempDir()

	// Build A (older) and Build B (newer) with different covmeta hashes
	dirA := filepath.Join(root, "build-a")
	dirB := filepath.Join(root, "build-b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Build A: older timestamp
	if err := os.WriteFile(filepath.Join(dirA, "covmeta.aaa"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	counterA := filepath.Join(dirA, "covcounters.aaa")
	if err := os.WriteFile(counterA, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	olderTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(counterA, olderTime, olderTime); err != nil {
		t.Fatal(err)
	}

	// Build B: newer timestamp
	if err := os.WriteFile(filepath.Join(dirB, "covmeta.bbb"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	counterB := filepath.Join(dirB, "covcounters.bbb")
	if err := os.WriteFile(counterB, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	newerTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(counterB, newerTime, newerTime); err != nil {
		t.Fatal(err)
	}

	groups, err := ParseDirRecursiveGrouped(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// First group should be older (build A), last should be newer (build B)
	if !groups[0].NewestTimestamp.Before(groups[1].NewestTimestamp) {
		t.Errorf("groups not sorted by timestamp: [0]=%v [1]=%v",
			groups[0].NewestTimestamp, groups[1].NewestTimestamp)
	}

	// Verify build A is first (has dirA)
	if groups[0].Dirs[0] != dirA {
		t.Errorf("expected first group to contain %s, got %v", dirA, groups[0].Dirs)
	}
	// Verify build B is last (has dirB)
	if groups[1].Dirs[0] != dirB {
		t.Errorf("expected second group to contain %s, got %v", dirB, groups[1].Dirs)
	}
}
