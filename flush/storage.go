package flush

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Storage abstracts the destination for coverage data files.
type Storage interface {
	// Store saves coverage files (covmeta + covcounters) from a temporary directory.
	// files is a list of file paths within the temp directory.
	Store(ctx context.Context, files []string, meta Metadata) error
}

// Metadata carries information associated with coverage data.
type Metadata struct {
	Timestamp    time.Time
	Hostname     string
	PodName      string // auto-populated from POD_NAME env var (k8s downward API)
	BuildVersion string // build version or commit hash (set via Config)
	ServiceName  string // service identifier (set via Config)
}

// LocalStorage saves coverage files to a local directory in GOCOVERDIR-compatible layout.
type LocalStorage struct {
	Dir string
}

func (s LocalStorage) Store(_ context.Context, files []string, _ Metadata) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("goreach/flush: mkdir %s: %w", s.Dir, err)
	}
	for _, src := range files {
		dst := filepath.Join(s.Dir, filepath.Base(src))
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("goreach/flush: copy %s: %w", filepath.Base(src), err)
		}
	}
	return nil
}

// WriterStorage writes coverage data as text profile format to the given writer.
// This is primarily for debugging purposes.
type WriterStorage struct {
	W io.Writer
}

func (s WriterStorage) Store(_ context.Context, files []string, meta Metadata) error {
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("goreach/flush: read %s: %w", filepath.Base(f), err)
		}
		header := fmt.Sprintf("=== %s (service=%s version=%s host=%s) ===\n",
			filepath.Base(f), meta.ServiceName, meta.BuildVersion, meta.Hostname)
		if _, err := io.WriteString(s.W, header); err != nil {
			return err
		}
		if _, err := s.W.Write(data); err != nil {
			return err
		}
		if _, err := io.WriteString(s.W, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
