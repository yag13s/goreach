package main

import (
	"fmt"
	"os"

	"github.com/yag13s/goreach/internal/report"
)

// writeReport writes rpt as JSON to path, or to stdout if path is empty.
func writeReport(rpt *report.Report, path string, pretty bool) (err error) {
	if path == "" {
		return rpt.Write(os.Stdout, pretty)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("close output file: %w", cerr)
		}
	}()
	return rpt.Write(f, pretty)
}
