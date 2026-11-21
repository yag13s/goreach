package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yag13s/goreach/internal/report"
)

func parse(t *testing.T, args ...string) (*flag.FlagSet, *string) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	rpt := fs.String("report", "", "")
	fs.String("src", "", "")
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return fs, rpt
}

func TestReportPathArg(t *testing.T) {
	tests := []struct {
		args    []string
		want    string
		wantErr string
	}{
		{args: nil, want: ""},
		{args: []string{"r.json"}, want: "r.json"},
		{args: []string{"-report", "r.json"}, want: "r.json"},
		{args: []string{"-src", ".", "r.json"}, want: "r.json"},
		{args: []string{"r.json", "-src", "."}, wantErr: "must come before"},
		{args: []string{"a.json", "b.json"}, wantErr: "expected one report path"},
		{args: []string{"-report", "a.json", "b.json"}, wantErr: "not both"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			fs, rpt := parse(t, tt.args...)
			got, err := reportPathArg(fs, *rpt)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestNoPositionalArgs(t *testing.T) {
	fs, _ := parse(t, "-src", ".")
	if err := noPositionalArgs(fs); err != nil {
		t.Errorf("no positional args: %v", err)
	}
	fs, _ = parse(t, "extra")
	if err := noPositionalArgs(fs); err == nil {
		t.Error("extra argument accepted")
	}
}

func TestSummaryRejectsRWithoutCoverdir(t *testing.T) {
	if err := runSummary([]string{"-r", "-profile", "x.out"}); err == nil || !strings.Contains(err.Error(), "-r requires -coverdir") {
		t.Errorf("err = %v, want -r requires -coverdir", err)
	}
}

func TestWriteReport_ReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeReport(&report.Report{Version: 1}, path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := report.ReadFile(path); err != nil {
		t.Fatalf("written report unreadable: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want only report.json (no temp files left)", len(entries))
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
	}
}
