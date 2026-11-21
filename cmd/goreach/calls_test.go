package main

import (
	"strings"
	"testing"

	"github.com/yag13s/goreach/internal/report"
)

func callsReport() *report.Report {
	n := func(v int) *int { return &v }
	return &report.Report{Version: report.SchemaVersion, Packages: []report.PackageReport{
		{ImportPath: "example.com/app/handler", Files: []report.FileReport{{
			FileName: "example.com/app/handler/user.go",
			Functions: []report.FuncReport{
				{Name: "(*User).Get", Line: 10, TotalStatements: 5, Calls: n(1234567)},
				{Name: "(*User).Export", Line: 40, TotalStatements: 14, Calls: n(0)},
				{Name: "(*User).List", Line: 60, TotalStatements: 8, Calls: n(42)},
			},
		}}},
		{ImportPath: "example.com/app/store", Files: []report.FileReport{{
			FileName: "example.com/app/store/db.go",
			Functions: []report.FuncReport{
				{Name: "Query", Line: 7, TotalStatements: 3, Calls: n(2000000)},
				{Name: "Unknown", Line: 30, TotalStatements: 3},
			},
		}}},
	}}
}

func TestPrintCalls(t *testing.T) {
	var b strings.Builder
	if err := printCalls(&b, callsReport(), nil, 2, true); err != nil {
		t.Fatal(err)
	}
	want := `Most Called Functions (top 2 of 3 called)
=========================================

      calls  function
  2,000,000  example.com/app/store/db.go:7  Query
  1,234,567  example.com/app/handler/user.go:10  (*User).Get

Never Called Functions (1 of 4, 14 statements)
==============================================

  example.com/app/handler/user.go:40  (*User).Export  (14 stmts)
`
	if got := b.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintCalls_PkgFilter(t *testing.T) {
	var b strings.Builder
	if err := printCalls(&b, callsReport(), []string{"example.com/app/handler"}, 10, false); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); strings.Contains(got, "Query") || !strings.Contains(got, "(*User).List") {
		t.Errorf("filter not applied:\n%s", got)
	}
	if err := printCalls(&b, callsReport(), []string{"nope"}, 10, false); err == nil || !strings.Contains(err.Error(), "-pkg") {
		t.Errorf("err = %v, want one naming -pkg", err)
	}
}

func TestPrintCalls_NoCounts(t *testing.T) {
	rpt := &report.Report{Packages: []report.PackageReport{{Files: []report.FileReport{{
		Functions: []report.FuncReport{{Name: "F"}},
	}}}}}
	var b strings.Builder
	if err := printCalls(&b, rpt, nil, 5, false); err == nil || !strings.Contains(err.Error(), "no call counts") {
		t.Errorf("err = %v, want one saying the report has no call counts", err)
	}
}

func TestGroupDigits(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 123456: "123,456", 1234567: "1,234,567"} {
		if got := groupDigits(n); got != want {
			t.Errorf("groupDigits(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestSummary_CallFlagsNeedReport(t *testing.T) {
	if err := runSummary([]string{"-top", "5", "-profile", "x.out"}); err == nil || !strings.Contains(err.Error(), "need a report") {
		t.Errorf("err = %v, want one saying -top needs a report", err)
	}
	if err := runSummary([]string{"-pkg", "x", "r.json"}); err == nil || !strings.Contains(err.Error(), "-pkg applies") {
		t.Errorf("err = %v, want one about -pkg", err)
	}
}
