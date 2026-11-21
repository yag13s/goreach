// Package astmap extracts function declarations and their source positions from Go files.
package astmap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"strconv"
	"strings"
)

// FuncExtent describes the source position of a function declaration.
type FuncExtent struct {
	Name      string // function name with receiver, e.g. "(*Server).Handle"
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int

	// LoopCalls are the function calls made inside a for or range loop in
	// the function body, including inside function literals in the loop.
	// Builtins are left out; type conversions cannot be told apart from
	// calls without type information and are included.
	LoopCalls []CallSite
}

// CallSite is a call expression and what it names, as far as the syntax
// alone tells.
type CallSite struct {
	Line int
	Col  int

	// Name is the called function's or method's name, and Expr the called
	// expression as written, such as "h.repo.Get".
	Name string
	Expr string

	// TakesContext reports whether the first argument looks like a
	// context.Context: an identifier named ctx or ending in Ctx, or a call
	// to a Context method (r.Context()). Calls that take a context usually
	// do I/O.
	TakesContext bool

	// Pkg is the import path of the package when the call is pkg.Name(),
	// Method reports a call x.Name() on anything else, and with neither set
	// the call is Name(), a function of the same package.
	Pkg    string
	Method bool
}

// FileFuncs parses the given Go source file and returns the function declarations it contains.
func FileFuncs(filename string) ([]*FuncExtent, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("astmap: parse %s: %w", filename, err)
	}
	imports := importNames(f)

	var funcs []*FuncExtent
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
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
			LoopCalls: loopCalls(fset, fn.Body, imports),
		})
	}
	return funcs, nil
}

// importNames maps the names a file refers to its imports by to their import
// paths. A package's name is taken to be the last element of its path, less
// a major version suffix ("gopkg.in/yaml.v3" → yaml, "x/v2" → x's parent).
func importNames(f *ast.File) map[string]string {
	names := make(map[string]string, len(f.Imports))
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		var name string
		if imp.Name != nil {
			name = imp.Name.Name
		} else {
			name = defaultImportName(p)
		}
		if name == "_" || name == "." {
			continue
		}
		names[name] = p
	}
	return names
}

func defaultImportName(importPath string) string {
	name := path.Base(importPath)
	if isMajorVersion(name) && path.Dir(importPath) != "." {
		name = path.Base(path.Dir(importPath))
	}
	if i := strings.Index(name, ".v"); i > 0 && isMajorVersion(name[i+1:]) {
		name = name[:i]
	}
	return strings.ReplaceAll(name, "-", "_")
}

// isMajorVersion reports whether s is "v" followed by digits.
func isMajorVersion(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	_, err := strconv.Atoi(s[1:])
	return err == nil
}

var builtins = map[string]bool{
	"append": true, "cap": true, "clear": true, "close": true, "complex": true,
	"copy": true, "delete": true, "imag": true, "len": true, "make": true,
	"max": true, "min": true, "new": true, "panic": true, "print": true,
	"println": true, "real": true, "recover": true,
}

// loopCalls returns the calls made inside loops in body.
func loopCalls(fset *token.FileSet, body *ast.BlockStmt, imports map[string]string) []CallSite {
	var sites []CallSite
	var walk func(n ast.Node, inLoop bool)
	walk = func(n ast.Node, inLoop bool) {
		ast.Inspect(n, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ForStmt:
				// The init statement runs once; everything else per iteration.
				if n.Init != nil {
					walk(n.Init, inLoop)
				}
				// A missing Cond or Post is a nil interface here: Cond is
				// an ast.Expr and Post an ast.Stmt, both nil when absent.
				for _, c := range []ast.Node{n.Cond, n.Post, n.Body} {
					if c != nil {
						walk(c, true)
					}
				}
				return false
			case *ast.RangeStmt:
				// The range expression is evaluated once.
				walk(n.X, inLoop)
				walk(n.Body, true)
				return false
			case *ast.CallExpr:
				if inLoop {
					if site, ok := callSite(fset, n, imports); ok {
						sites = append(sites, site)
					}
				}
			}
			return true
		})
	}
	walk(body, false)
	return sites
}

func callSite(fset *token.FileSet, call *ast.CallExpr, imports map[string]string) (CallSite, bool) {
	fun := ast.Unparen(call.Fun)
	// Explicit instantiation: F[T](x), F[T, U](x).
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}
	pos := fset.Position(call.Lparen)
	site := CallSite{
		Line:         pos.Line,
		Col:          pos.Column,
		Expr:         types.ExprString(fun),
		TakesContext: len(call.Args) > 0 && isContextArg(call.Args[0]),
	}
	switch f := fun.(type) {
	case *ast.Ident:
		if builtins[f.Name] {
			return CallSite{}, false
		}
		site.Name = f.Name
	case *ast.SelectorExpr:
		site.Name = f.Sel.Name
		if x, ok := f.X.(*ast.Ident); ok && imports[x.Name] != "" {
			site.Pkg = imports[x.Name]
		} else {
			site.Method = true
		}
	default:
		// A call of a function value: f()(), funcs[i](), func(){}().
		return CallSite{}, false
	}
	return site, true
}

func isContextArg(arg ast.Expr) bool {
	switch a := ast.Unparen(arg).(type) {
	case *ast.Ident:
		return a.Name == "ctx" || strings.HasSuffix(a.Name, "Ctx")
	case *ast.CallExpr:
		sel, ok := a.Fun.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "Context" && len(a.Args) == 0
	}
	return false
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
		indices := make([]string, len(t.Indices))
		for i, idx := range t.Indices {
			indices[i] = exprString(idx)
		}
		return exprString(t.X) + "[" + strings.Join(indices, ", ") + "]"
	default:
		return fmt.Sprintf("%T", expr)
	}
}
