package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/cover"

	"github.com/yag13s/goreach/internal/report"
)

func TestSummarizeProfiles(t *testing.T) {
	profiles := []*cover.Profile{
		{FileName: "example.com/app/sub/z.go", Blocks: []cover.ProfileBlock{
			{NumStmt: 4, Count: 0},
		}},
		{FileName: "example.com/app/a.go", Blocks: []cover.ProfileBlock{
			{NumStmt: 2, Count: 3},
			{NumStmt: 1, Count: 0},
		}},
		{FileName: "example.com/app/b.go", Blocks: []cover.ProfileBlock{
			{NumStmt: 5, Count: 1},
		}},
	}

	got := summarizeProfiles(profiles)
	want := []summaryRow{
		{pkg: "example.com/app", covered: 7, total: 8},
		{pkg: "example.com/app/sub", covered: 0, total: 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSummarizeReport(t *testing.T) {
	// Package totals are taken as recorded: they may cover more than the
	// functions listed (filtered report), and the report need not be sorted.
	rpt := &report.Report{
		Total: report.NewCoverageStats(13, 20),
		Packages: []report.PackageReport{
			{ImportPath: "example.com/app/sub", Total: report.NewCoverageStats(9, 13)},
			{ImportPath: "example.com/app", Total: report.NewCoverageStats(4, 7)},
		},
	}

	got := summarizeReport(rpt)
	want := []summaryRow{
		{pkg: "example.com/app", covered: 4, total: 7},
		{pkg: "example.com/app/sub", covered: 9, total: 13},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	err := printSummary(&buf, []summaryRow{
		{pkg: "example.com/app", covered: 4, total: 7},
		{pkg: "example.com/app/sub", covered: 9, total: 13},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "Coverage Summary\n" +
		"================\n" +
		"\n" +
		"  example.com/app                                               57.1% (4/7)\n" +
		"  example.com/app/sub                                           69.2% (9/13)\n" +
		"\n" +
		"  TOTAL                                                         65.0% (13/20)\n"
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintSummary_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := printSummary(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "0.0% (0/0)\n") {
		t.Errorf("expected a zero TOTAL line, got:\n%s", buf.String())
	}
}
