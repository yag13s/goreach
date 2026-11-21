package analysis

import (
	"reflect"
	"testing"

	"github.com/yag13s/goreach/internal/astmap"
	"github.com/yag13s/goreach/internal/report"
)

func TestLoopCallCounts(t *testing.T) {
	blocks := []report.Block{
		{StartLine: 10, StartCol: 1, EndLine: 12, EndCol: 2, Count: 3},   // function body
		{StartLine: 12, StartCol: 20, EndLine: 15, EndCol: 3, Count: 60}, // loop body
		{StartLine: 14, StartCol: 10, EndLine: 14, EndCol: 30, Count: 7}, // func literal inside
		{StartLine: 16, StartCol: 2, EndLine: 17, EndCol: 3, Count: 0},   // loop never entered
	}
	sites := []astmap.CallSite{
		{Line: 13, Col: 5, Expr: "s.Get"},
		{Line: 13, Col: 25, Expr: "s.Get"}, // same call twice on a line
		{Line: 14, Col: 15, Expr: "lookup"},
		{Line: 16, Col: 5, Expr: "never"},
		{Line: 99, Col: 1, Expr: "nowhere"},
	}
	got := loopCallCounts(sites, blocks, 3)
	want := pendingCalls{
		{site: sites[0], executions: 60, callerCalls: 3},
		{site: sites[2], executions: 7, callerCalls: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestResolveLoopCalls(t *testing.T) {
	n := func(v int) *int { return &v }
	pkgs := []report.PackageReport{
		{ImportPath: "app/handler", Files: []report.FileReport{{
			FileName: "app/handler/h.go",
			Functions: []report.FuncReport{
				{Name: "List", Calls: n(3)},
				{Name: "helper", Calls: n(0)},
			},
		}}},
		{ImportPath: "app/repo", Files: []report.FileReport{{
			FileName: "app/repo/r.go",
			Functions: []report.FuncReport{
				{Name: "(*Likes).Has", Calls: n(60)},
				{Name: "(*Fake).Has", Calls: n(0)},
				{Name: "(*A).Get", Calls: n(1)},
				{Name: "(*B).Get", Calls: n(1)},
				{Name: "Open", Calls: n(1)},
			},
		}}},
	}
	call := func(line int, site astmap.CallSite, exec int) pendingCall {
		site.Line = line
		return pendingCall{site: site, executions: exec, callerCalls: 3}
	}
	pending := [][][]pendingCalls{
		{{{
			call(10, astmap.CallSite{Name: "Has", Expr: "h.likes.Has", Method: true, TakesContext: true}, 60),
			call(11, astmap.CallSite{Name: "Get", Expr: "x.Get", Method: true}, 90),                  // ambiguous, kept: own code
			call(12, astmap.CallSite{Name: "Open", Expr: "repo.Open", Pkg: "app/repo"}, 6),           // package function
			call(13, astmap.CallSite{Name: "helper", Expr: "helper"}, 6),                             // same package
			call(14, astmap.CallSite{Name: "ToUpper", Expr: "strings.ToUpper", Pkg: "strings"}, 900), // not ours: dropped
			call(15, astmap.CallSite{Name: "GetItem", Expr: "db.GetItem", Method: true, TakesContext: true}, 30),
		}, nil}},
		{{nil, nil, nil, nil, nil}},
	}
	resolveLoopCalls(pkgs, pending)

	got := pkgs[0].Files[0].Functions[0].LoopCalls
	want := []report.LoopCall{
		{Line: 11, Expr: "x.Get", Executions: 90, CallerCalls: 3},
		{Line: 10, Expr: "h.likes.Has", CalleeFile: "app/repo/r.go", Callee: "(*Likes).Has", TakesContext: true, Executions: 60, CallerCalls: 3},
		{Line: 15, Expr: "db.GetItem", TakesContext: true, Executions: 30, CallerCalls: 3},
		{Line: 12, Expr: "repo.Open", CalleeFile: "app/repo/r.go", Callee: "Open", Executions: 6, CallerCalls: 3},
		{Line: 13, Expr: "helper", CalleeFile: "app/handler/h.go", Callee: "helper", Executions: 6, CallerCalls: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%+v\nwant:\n%+v", got, want)
	}
}
