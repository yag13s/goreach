package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadFile(t *testing.T) {
	r := &Report{
		Version:     1,
		GeneratedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Mode:        "set",
		Total: CoverageStats{
			TotalStatements:   100,
			CoveredStatements: 75,
			CoveragePercent:   75.0,
		},
		Packages: []PackageReport{
			{
				ImportPath: "example.com/pkg",
				Total: CoverageStats{
					TotalStatements:   100,
					CoveredStatements: 75,
					CoveragePercent:   75.0,
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Mode != "set" {
		t.Errorf("Mode = %q, want %q", got.Mode, "set")
	}
	if got.Total.CoveragePercent != 75.0 {
		t.Errorf("Total.CoveragePercent = %v, want 75.0", got.Total.CoveragePercent)
	}
	if len(got.Packages) != 1 {
		t.Errorf("len(Packages) = %d, want 1", len(got.Packages))
	}
}

func TestReadFileNotFound(t *testing.T) {
	_, err := ReadFile("/nonexistent/path/report.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestComputePercent(t *testing.T) {
	tests := []struct {
		covered, total int
		want           float64
	}{
		{0, 0, 0},
		{0, 100, 0},
		{50, 100, 50},
		{100, 100, 100},
		{1, 3, 33.33333333333333},
	}
	for _, tt := range tests {
		got := ComputePercent(tt.covered, tt.total)
		if got != tt.want {
			t.Errorf("ComputePercent(%d, %d) = %v, want %v", tt.covered, tt.total, got, tt.want)
		}
	}
}

func TestReportWrite(t *testing.T) {
	r := &Report{
		Version:     1,
		GeneratedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Mode:        "set",
		Total: CoverageStats{
			TotalStatements:   100,
			CoveredStatements: 75,
			CoveragePercent:   75.0,
		},
		Packages: []PackageReport{
			{
				ImportPath: "example.com/pkg",
				Total: CoverageStats{
					TotalStatements:   100,
					CoveredStatements: 75,
					CoveragePercent:   75.0,
				},
				Files: []FileReport{
					{
						FileName: "example.com/pkg/foo.go",
						Total: CoverageStats{
							TotalStatements:   100,
							CoveredStatements: 75,
							CoveragePercent:   75.0,
						},
						Functions: []FuncReport{
							{
								Name:              "Foo",
								Line:              10,
								TotalStatements:   25,
								CoveredStatements: 0,
								CoveragePercent:   0,
								UnreachedBlocks: []UnreachedBlock{
									{StartLine: 11, StartCol: 2, EndLine: 33, EndCol: 3, NumStatements: 25},
								},
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := r.Write(&buf, false); err != nil {
		t.Fatal(err)
	}

	// Verify it's valid JSON
	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if decoded.Version != 1 {
		t.Errorf("version = %d, want 1", decoded.Version)
	}
	if decoded.Total.CoveragePercent != 75.0 {
		t.Errorf("total coverage = %v, want 75.0", decoded.Total.CoveragePercent)
	}
	if len(decoded.Packages) != 1 {
		t.Errorf("packages count = %d, want 1", len(decoded.Packages))
	}

	// Test pretty output
	var prettyBuf bytes.Buffer
	if err := r.Write(&prettyBuf, true); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(prettyBuf.Bytes(), []byte("  ")) {
		t.Error("pretty output should contain indentation")
	}
}

func TestNewCoverageStats(t *testing.T) {
	got := NewCoverageStats(3, 4)
	want := CoverageStats{TotalStatements: 4, CoveredStatements: 3, CoveragePercent: 75}
	if got != want {
		t.Errorf("NewCoverageStats(3, 4) = %+v, want %+v", got, want)
	}
	if got := NewCoverageStats(0, 0); got != (CoverageStats{}) {
		t.Errorf("NewCoverageStats(0, 0) = %+v, want zero value", got)
	}
}

func TestClone(t *testing.T) {
	orig := &Report{
		Version:     SchemaVersion,
		GeneratedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Mode:        "atomic",
		Total:       NewCoverageStats(1, 4),
		Packages: []PackageReport{{
			ImportPath: "example.com/pkg",
			Total:      NewCoverageStats(1, 4),
			Files: []FileReport{{
				FileName: "example.com/pkg/foo.go",
				Total:    NewCoverageStats(1, 4),
				Functions: []FuncReport{{
					Name: "Foo", Line: 10,
					TotalStatements: 4, CoveredStatements: 1, CoveragePercent: 25,
					UnreachedBlocks:       []UnreachedBlock{{StartLine: 11, EndLine: 13, NumStatements: 3}},
					LatestUnreachedBlocks: []UnreachedBlock{{StartLine: 21, EndLine: 23, NumStatements: 3}},
				}},
			}},
		}},
	}

	clone := orig.Clone()
	if !reflect.DeepEqual(orig, clone) {
		t.Fatalf("clone differs from original:\n got %+v\nwant %+v", clone, orig)
	}

	// Mutating every level of the clone must leave the original untouched.
	clone.Mode = ModeMerged
	clone.Packages[0].ImportPath = "changed"
	clone.Packages[0].Files[0].FileName = "changed"
	fn := &clone.Packages[0].Files[0].Functions[0]
	fn.Name = "changed"
	fn.UnreachedBlocks[0].StartLine = 99
	fn.LatestUnreachedBlocks[0].StartLine = 99

	origFn := orig.Packages[0].Files[0].Functions[0]
	if orig.Mode != "atomic" ||
		orig.Packages[0].ImportPath != "example.com/pkg" ||
		orig.Packages[0].Files[0].FileName != "example.com/pkg/foo.go" ||
		origFn.Name != "Foo" ||
		origFn.UnreachedBlocks[0].StartLine != 11 ||
		origFn.LatestUnreachedBlocks[0].StartLine != 21 {
		t.Errorf("mutating the clone changed the original: %+v", orig)
	}
}

func TestClone_EmptyListsEncodeAsArrays(t *testing.T) {
	var buf bytes.Buffer
	if err := (&Report{Version: SchemaVersion}).Clone().Write(&buf, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"packages":[]`) {
		t.Errorf("expected packages to encode as [], got %s", buf.String())
	}
}
