package flush

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Storage abstracts the destination for coverage data files.
type Storage interface {
	// Store saves coverage files (covmeta + covcounters). files are paths to
	// temporary files that are removed once Store returns; they need not
	// share a directory, and each is to be stored under its base name. With
	// Config.Clear, counter files of earlier flushes whose Store failed are
	// included again, so storing a file must be safe to repeat.
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
	if s.W == nil {
		return fmt.Errorf("goreach/flush: WriterStorage.W is nil")
	}
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

// copyFile copies src to dst atomically: it writes a temporary file next to
// dst and renames it into place, so a failed copy never leaves a truncated
// file and a concurrent reader (goreach analyze) never sees a partial one.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// O_EXCL with mode 0666 rather than os.CreateTemp, which would create the
	// file 0600: the result should get the permissions os.Create gives it,
	// umask applied.
	tmp := filepath.Join(filepath.Dir(dst), ".goreach-tmp-"+rand.Text())
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666) //nolint:gosec // same mode as os.Create; the umask decides
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
