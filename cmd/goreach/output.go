package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yag13s/goreach/internal/report"
)

// writeReport writes rpt as JSON to path, or to stdout if path is empty.
//
// The file is written to a temporary file in the same directory and renamed
// into place, so a failed write never leaves a truncated report, and
// `merge -o a.json a.json b.json` cannot destroy an input.
func writeReport(rpt *report.Report, path string, pretty bool) (err error) {
	if path == "" {
		return rpt.Write(os.Stdout, pretty)
	}

	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()

	if err := rpt.Write(f, pretty); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}
	// CreateTemp uses 0600; match what os.Create would have produced.
	if err := os.Chmod(tmp, 0o644); err != nil { //nolint:gosec // reports are not secret
		return fmt.Errorf("set output file mode: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}
