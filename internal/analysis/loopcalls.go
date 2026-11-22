package analysis

import (
	"cmp"
	"slices"
	"strings"

	"github.com/yag13s/goreach/internal/astmap"
	"github.com/yag13s/goreach/internal/report"
)

// pendingCall is a call made inside a loop whose execution count is known
// but whose callee has not been looked up in the report yet.
type pendingCall struct {
	site        astmap.CallSite
	executions  int
	callerCalls int
}

// pendingCalls are the loop calls of one function.
type pendingCalls []pendingCall

// loopCallCounts pairs each call site with the execution count of the
// coverage block it lies in, dropping calls that never ran. callerCalls is
// how many times the function was entered.
func loopCallCounts(sites []astmap.CallSite, blocks []report.Block, callerCalls int) pendingCalls {
	var calls pendingCalls
	type key struct {
		line int
		expr string
	}
	seen := make(map[key]bool)
	for _, site := range sites {
		b, ok := blockAt(blocks, site.Line, site.Col)
		if !ok || b.Count == 0 {
			continue
		}
		// The same call twice on a line, as in f(x) + f(y), is one row.
		k := key{site.Line, site.Expr}
		if seen[k] {
			continue
		}
		seen[k] = true
		calls = append(calls, pendingCall{site: site, executions: b.Count, callerCalls: callerCalls})
	}
	return calls
}

// blockAt returns the smallest block containing line:col.
func blockAt(blocks []report.Block, line, col int) (report.Block, bool) {
	var best report.Block
	found := false
	for _, b := range blocks {
		if !posLE(b.StartLine, b.StartCol, line, col) || !posLE(line, col, b.EndLine, b.EndCol) {
			continue
		}
		if !found || posLE(best.StartLine, best.StartCol, b.StartLine, b.StartCol) &&
			posLE(b.EndLine, b.EndCol, best.EndLine, best.EndCol) {
			best, found = b, true
		}
	}
	return best, found
}

// posLE reports whether line1:col1 is at or before line2:col2.
func posLE(line1, col1, line2, col2 int) bool {
	return line1 < line2 || line1 == line2 && col1 <= col2
}

// funcRef identifies a function of the report.
type funcRef struct {
	file  string
	name  string
	calls int
}

// calleeIndex finds the report's functions by the names calls use.
type calleeIndex struct {
	funcs   map[[2]string][]funcRef // {import path, name} → plain functions
	methods map[string][]funcRef    // method name → methods
}

func newCalleeIndex(pkgs []report.PackageReport) *calleeIndex {
	idx := &calleeIndex{
		funcs:   make(map[[2]string][]funcRef),
		methods: make(map[string][]funcRef),
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, fn := range file.Functions {
				ref := funcRef{file: file.FileName, name: fn.Name}
				if fn.Calls != nil {
					ref.calls = *fn.Calls
				}
				if i := strings.LastIndex(fn.Name, ")."); strings.HasPrefix(fn.Name, "(") && i > 0 {
					m := fn.Name[i+2:]
					idx.methods[m] = append(idx.methods[m], ref)
				} else {
					k := [2]string{pkg.ImportPath, fn.Name}
					idx.funcs[k] = append(idx.funcs[k], ref)
				}
			}
		}
	}
	return idx
}

// candidates returns the functions a call made from package pkg may call.
func (idx *calleeIndex) candidates(pkg string, site astmap.CallSite) []funcRef {
	switch {
	case site.Method:
		return idx.methods[site.Name]
	case site.Pkg != "":
		return idx.funcs[[2]string{site.Pkg, site.Name}]
	default:
		return idx.funcs[[2]string{pkg, site.Name}]
	}
}

// resolve returns the function a call calls, if exactly one candidate fits.
// Among several, a single one that was ever called is taken to be it: an
// interface usually has one implementation running in production, the rest
// being test doubles or alternatives.
func resolve(cands []funcRef) (funcRef, bool) {
	if len(cands) == 1 {
		return cands[0], true
	}
	var called []funcRef
	for _, c := range cands {
		if c.calls > 0 {
			called = append(called, c)
		}
	}
	if len(called) == 1 {
		return called[0], true
	}
	return funcRef{}, false
}

// resolveLoopCalls looks up the callee of each pending loop call and records
// the calls worth reporting on their functions: those to functions in the
// report and those taking a context. pending is aligned with pkgs, their
// files and their functions.
func resolveLoopCalls(pkgs []report.PackageReport, pending [][][]pendingCalls) {
	idx := newCalleeIndex(pkgs)
	for i := range pkgs {
		for j := range pkgs[i].Files {
			file := &pkgs[i].Files[j]
			for k := range file.Functions {
				fn := &file.Functions[k]
				for _, p := range pending[i][j][k] {
					cands := idx.candidates(pkgs[i].ImportPath, p.site)
					if len(cands) == 0 && !p.site.TakesContext {
						continue
					}
					lc := report.LoopCall{
						Line:         p.site.Line,
						Expr:         p.site.Expr,
						TakesContext: p.site.TakesContext,
						Executions:   p.executions,
						CallerCalls:  p.callerCalls,
					}
					if ref, ok := resolve(cands); ok {
						lc.CalleeFile, lc.Callee = ref.file, ref.name
					}
					fn.LoopCalls = append(fn.LoopCalls, lc)
				}
				slices.SortStableFunc(fn.LoopCalls, func(a, b report.LoopCall) int {
					return cmp.Or(cmp.Compare(b.Executions, a.Executions), cmp.Compare(a.Line, b.Line))
				})
			}
		}
	}
}
