package astmap

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func testdataDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "testdata", "sample_source")
}

func TestFileFuncs(t *testing.T) {
	funcs, err := FileFuncs(filepath.Join(testdataDir(), "sample.go"))
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]bool{
		"Add":                    false,
		"Subtract":               false,
		"Greet":                  false,
		"(*Calculator).Add":      false,
		"(*Calculator).Multiply": false,
		"neverCalled":            false,
	}

	for _, fn := range funcs {
		if _, ok := expected[fn.Name]; ok {
			expected[fn.Name] = true
		} else {
			t.Errorf("unexpected function: %s", fn.Name)
		}

		if fn.StartLine == 0 || fn.EndLine == 0 {
			t.Errorf("function %s has zero line number", fn.Name)
		}
		if fn.EndLine < fn.StartLine {
			t.Errorf("function %s: end line %d < start line %d", fn.Name, fn.EndLine, fn.StartLine)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected function not found: %s", name)
		}
	}
}

func TestFileFuncs_BadFile(t *testing.T) {
	_, err := FileFuncs("/nonexistent/file.go")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// TestFileFuncs_Generics tests parsing of generic type receivers, which
// exercise the IndexExpr (single type param) and IndexListExpr (multiple
// type params) branches of exprString.
func TestFileFuncs_Generics(t *testing.T) {
	funcs, err := FileFuncs(filepath.Join(testdataDir(), "generics.go"))
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]bool{
		// Container[T] has one type param -> IndexExpr
		"(Container[T]).Get":  false,
		"(*Container[T]).Set": false,
		// Pair[K, V] has multiple type params -> IndexListExpr
		"(Pair[K, V]).GetKey":  false,
		"(*Pair[K, V]).SetKey": false,
	}

	for _, fn := range funcs {
		if _, ok := expected[fn.Name]; ok {
			expected[fn.Name] = true
		} else {
			t.Errorf("unexpected function: %s", fn.Name)
		}

		if fn.StartLine == 0 || fn.EndLine == 0 {
			t.Errorf("function %s has zero line number", fn.Name)
		}
		if fn.EndLine < fn.StartLine {
			t.Errorf("function %s: end line %d < start line %d", fn.Name, fn.EndLine, fn.StartLine)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected function not found: %s", name)
		}
	}
}

// TestFileFuncs_Interface tests that bodyless functions (interface method
// declarations) are skipped without panicking.
func TestFileFuncs_Interface(t *testing.T) {
	funcs, err := FileFuncs(filepath.Join(testdataDir(), "interface.go"))
	if err != nil {
		t.Fatal(err)
	}

	// Only concrete methods should be returned, not interface method declarations.
	expected := map[string]bool{
		"(*ConcreteRunner).Run":  false,
		"(*ConcreteRunner).Stop": false,
	}

	for _, fn := range funcs {
		if _, ok := expected[fn.Name]; ok {
			expected[fn.Name] = true
		} else {
			t.Errorf("unexpected function (possible bodyless leak): %s", fn.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected function not found: %s", name)
		}
	}
}

// TestExprString_Default tests the default branch of exprString which
// handles unknown/unsupported expression types by returning the Go type name.
func TestExprString_Default(t *testing.T) {
	// Use an expression type that is not handled by any specific case.
	// *ast.CompositeLit is a valid ast.Expr but not handled by exprString.
	expr := &ast.CompositeLit{}
	result := exprString(expr)
	want := "*ast.CompositeLit"
	if result != want {
		t.Errorf("exprString(CompositeLit) = %q, want %q", result, want)
	}

	// Also test with another unhandled type: *ast.CallExpr
	callExpr := &ast.CallExpr{}
	result = exprString(callExpr)
	want = "*ast.CallExpr"
	if result != want {
		t.Errorf("exprString(CallExpr) = %q, want %q", result, want)
	}
}

func TestFileFuncs_LoopCalls(t *testing.T) {
	funcs, err := FileFuncs(filepath.Join(testdataDir(), "loops.go"))
	if err != nil {
		t.Fatal(err)
	}
	var got []CallSite
	for _, fn := range funcs {
		if fn.Name == "loops" {
			got = fn.LoopCalls
		}
	}
	for i := range got {
		got[i].Col = 0
	}
	want := []CallSite{
		{Line: 19, Name: "Get", Expr: "s.Get", Method: true, TakesContext: true},
		{Line: 21, Name: "ToUpper", Expr: "strings.ToUpper", Pkg: "strings"},
		{Line: 22, Name: "lookup", Expr: "lookup"},
		{Line: 24, Name: "count", Expr: "count"},
		{Line: 27, Name: "next", Expr: "next"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoopCalls:\n got %+v\nwant %+v", got, want)
	}
}

func TestDefaultImportName(t *testing.T) {
	for p, want := range map[string]string{
		"strings":                "strings",
		"gopkg.in/yaml.v3":       "yaml",
		"github.com/x/y/v2":      "y",
		"github.com/x/go-thing":  "go_thing",
		"example.com/pkg/v1beta": "v1beta",
	} {
		if got := defaultImportName(p); got != want {
			t.Errorf("defaultImportName(%q) = %q, want %q", p, got, want)
		}
	}
}
