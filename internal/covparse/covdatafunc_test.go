package covparse

import "testing"

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
