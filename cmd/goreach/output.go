package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yag13s/goreach/internal/report"
)

// writeReport writes rpt as JSON to path, or to stdout if path is empty.
//
// A regular file is written to a temporary file in the same directory and
// renamed into place, so a failed write never leaves a truncated report, and
// `merge -o a.json a.json b.json` cannot destroy an input. A symbolic link is
// followed, so the file it points to is replaced rather than the link.
// Anything else that already exists (/dev/stdout, /dev/null, a named pipe)
// is written to directly.
func writeReport(rpt *report.Report, path string, pretty bool) (err error) {
	if path == "" {
		return rpt.Write(os.Stdout, pretty)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	existing, statErr := os.Stat(path)
	if statErr == nil && !existing.Mode().IsRegular() {
		return writeReportTo(rpt, path, pretty)
	}

	// O_EXCL with mode 0666 rather than os.CreateTemp, which would create the
	// file 0600: a new report gets the permissions os.Create gives it.
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-"+rand.Text())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666) //nolint:gosec // same mode as os.Create; the umask decides
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
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
	// Overwriting in place would have kept the existing file's permissions.
	if statErr == nil {
		if err := os.Chmod(tmp, existing.Mode().Perm()); err != nil {
			return fmt.Errorf("set output file mode: %w", err)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

// writeReportTo writes rpt straight to path.
func writeReportTo(rpt *report.Report, path string, pretty bool) (err error) {
	// Write-only, unlike os.Create: /dev/stdout that is a pipe cannot be
	// opened for reading.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666) //nolint:gosec // same mode as os.Create
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
