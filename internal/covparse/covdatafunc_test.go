package covparse

import (
	"testing"

	"golang.org/x/tools/cover"
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

func TestParseCovdataFuncOutput_ModuleStartingWithTotal(t *testing.T) {
	out := "totally.dev/x/a.go:3:\t\tRun\t\t50.0%\ntotal\t\t\t\t(statements)\t50.0%\n"
	funcs := parseCovdataFuncOutput(out)
	if len(funcs) != 1 || funcs[0].FileName != "totally.dev/x/a.go" || funcs[0].FuncName != "Run" {
		t.Errorf("got %+v, want one Run in totally.dev/x/a.go", funcs)
	}
}

func TestParseCovdataFuncOutput_Line(t *testing.T) {
	funcs := parseCovdataFuncOutput("example.com/x/a.go:42:\t\tRun\t\t50.0%\n")
	if len(funcs) != 1 || funcs[0].Line != 42 {
		t.Errorf("got %+v, want Run at line 42", funcs)
	}
}

func TestAttachCalls(t *testing.T) {
	funcs := []FuncCoverage{
		{FileName: "example.com/x/a.go", FuncName: "Second", Line: 20},
		{FileName: "example.com/x/a.go", FuncName: "First", Line: 5},
		{FileName: "example.com/x/a.go", FuncName: "Empty", Line: 30}, // no block before Last
		{FileName: "example.com/x/a.go", FuncName: "Last", Line: 31},
		{FileName: "example.com/x/b.go", FuncName: "NoProfile", Line: 3},
		{FileName: "example.com/x/a.go", FuncName: "NoLine"},
		{FileName: "example.com/x/set.go", FuncName: "SetMode", Line: 3},
	}
	profiles := []*cover.Profile{
		{FileName: "example.com/x/a.go", Mode: "atomic", Blocks: []cover.ProfileBlock{
			{StartLine: 22, StartCol: 3, EndLine: 23, EndCol: 3, NumStmt: 1, Count: 1},
			{StartLine: 20, StartCol: 15, EndLine: 22, EndCol: 3, NumStmt: 1, Count: 9}, // Second's entry
			{StartLine: 5, StartCol: 14, EndLine: 7, EndCol: 2, NumStmt: 1, Count: 7},   // First's entry
			{StartLine: 31, StartCol: 13, EndLine: 33, EndCol: 2, NumStmt: 1, Count: 0}, // Last's entry
		}},
		{FileName: "example.com/x/set.go", Mode: "set", Blocks: []cover.ProfileBlock{
			{StartLine: 3, StartCol: 10, EndLine: 4, EndCol: 2, NumStmt: 1, Count: 1},
		}},
	}

	AttachCalls(funcs, profiles)

	want := map[string]int{"First": 7, "Second": 9, "Last": 0}
	for _, fc := range funcs {
		w, known := want[fc.FuncName]
		switch {
		case known && (fc.Calls == nil || *fc.Calls != w):
			t.Errorf("%s: calls = %v, want %d", fc.FuncName, fc.Calls, w)
		case !known && fc.Calls != nil:
			t.Errorf("%s: calls = %d, want unknown", fc.FuncName, *fc.Calls)
		}
	}
}

// TestAttachCalls_SameLine checks that functions declared on one line, which
// cannot be told apart by line, get no call count rather than a wrong one.
func TestAttachCalls_SameLine(t *testing.T) {
	funcs := []FuncCoverage{
		{FileName: "a.go", FuncName: "A", Line: 3},
		{FileName: "a.go", FuncName: "B", Line: 3},
	}
	AttachCalls(funcs, []*cover.Profile{{FileName: "a.go", Mode: "atomic", Blocks: []cover.ProfileBlock{
		{StartLine: 3, StartCol: 12, EndLine: 3, EndCol: 20, NumStmt: 1, Count: 4},
		{StartLine: 3, StartCol: 34, EndLine: 3, EndCol: 40, NumStmt: 1, Count: 6},
	}}})
	for _, fc := range funcs {
		if fc.Calls != nil {
			t.Errorf("%s: calls = %d, want unknown", fc.FuncName, *fc.Calls)
		}
	}
}
