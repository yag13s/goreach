package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

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
