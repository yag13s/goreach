// Package astmap extracts function declarations and their source positions from Go files.
package astmap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// FuncExtent describes the source position of a function declaration.
type FuncExtent struct {
	Name      string // function name with receiver, e.g. "(*Server).Handle"
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// FileFuncs parses the given Go source file and returns the function declarations it contains.
func FileFuncs(filename string) ([]*FuncExtent, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("astmap: parse %s: %w", filename, err)
	}

	var funcs []*FuncExtent
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		name := funcName(fn)
		start := fset.Position(fn.Body.Pos())
		end := fset.Position(fn.Body.End())

		funcs = append(funcs, &FuncExtent{
			Name:      name,
			StartLine: start.Line,
			StartCol:  start.Column,
			EndLine:   end.Line,
			EndCol:    end.Column,
		})
	}
	return funcs, nil
}

// funcName returns the qualified name of a function declaration.
// For methods, it includes the receiver type: "(*Type).Method" or "Type.Method".
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}

	recv := fn.Recv.List[0].Type
	return fmt.Sprintf("(%s).%s", exprString(recv), fn.Name.Name)
}

// exprString returns a simple string representation of a type expression.
func exprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return exprString(t.X) + "[" + exprString(t.Index) + "]"
	case *ast.IndexListExpr:
		s := exprString(t.X) + "["
		for i, idx := range t.Indices {
			if i > 0 {
				s += ", "
			}
			s += exprString(idx)
		}
		return s + "]"
	default:
		return fmt.Sprintf("%T", expr)
	}
}
