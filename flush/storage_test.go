package flush

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorage_Store(t *testing.T) {
	// Create temp source files to "upload"
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("meta-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "covcounters.abc"), []byte("counter-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(t.TempDir(), "output")
	storage := LocalStorage{Dir: dstDir}

	files := []string{
		filepath.Join(srcDir, "covmeta.abc"),
		filepath.Join(srcDir, "covcounters.abc"),
	}

	if err := storage.Store(context.Background(), files, Metadata{}); err != nil {
		t.Fatal(err)
	}

	// Verify files were copied
	data, err := os.ReadFile(filepath.Join(dstDir, "covmeta.abc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "meta-data" {
		t.Errorf("covmeta content = %q, want %q", data, "meta-data")
	}

	data, err = os.ReadFile(filepath.Join(dstDir, "covcounters.abc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "counter-data" {
		t.Errorf("covcounters content = %q, want %q", data, "counter-data")
	}
}

func TestWriterStorage_Store(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("meta"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	storage := WriterStorage{W: &buf}

	files := []string{filepath.Join(srcDir, "covmeta.abc")}
	meta := Metadata{
		ServiceName:  "test-svc",
		BuildVersion: "abc123",
		Hostname:     "host1",
	}

	if err := storage.Store(context.Background(), files, meta); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "covmeta.abc") {
		t.Error("output should contain filename")
	}
	if !strings.Contains(output, "test-svc") {
		t.Error("output should contain service name")
	}
	if !strings.Contains(output, "meta") {
		t.Error("output should contain file contents")
	}
}

func TestLocalStorage_EmptyFiles(t *testing.T) {
	dstDir := t.TempDir()
	storage := LocalStorage{Dir: dstDir}

	if err := storage.Store(context.Background(), nil, Metadata{}); err != nil {
		t.Fatal(err)
	}
}

// TestLocalStorage_Store_SourceNotFound tests that LocalStorage.Store returns an
// error when the source file does not exist.
func TestLocalStorage_Store_SourceNotFound(t *testing.T) {
	dstDir := t.TempDir()
	storage := LocalStorage{Dir: dstDir}

	files := []string{"/nonexistent/path/covmeta.abc"}
	err := storage.Store(context.Background(), files, Metadata{})
	if err == nil {
		t.Fatal("expected error for nonexistent source file")
	}
	if !strings.Contains(err.Error(), "goreach/flush") {
		t.Errorf("error should mention goreach/flush, got: %v", err)
	}
}

// TestLocalStorage_Store_InvalidDestDir tests that LocalStorage.Store returns
// an error when the destination directory path is invalid (e.g., path through
// a file, not a directory).
func TestLocalStorage_Store_InvalidDestDir(t *testing.T) {
	// Create a file where we expect a directory to be
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("I'm a file, not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Try to use a path through the file as the destination directory
	storage := LocalStorage{Dir: filepath.Join(blockingFile, "subdir")}

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := []string{filepath.Join(srcDir, "covmeta.abc")}
	err := storage.Store(context.Background(), files, Metadata{})
	if err == nil {
		t.Fatal("expected error for invalid destination directory")
	}
}

// TestWriterStorage_Store_FileNotFound tests that WriterStorage.Store returns
// an error when one of the files does not exist.
func TestWriterStorage_Store_FileNotFound(t *testing.T) {
	var buf bytes.Buffer
	storage := WriterStorage{W: &buf}

	files := []string{"/nonexistent/path/covmeta.abc"}
	err := storage.Store(context.Background(), files, Metadata{})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "goreach/flush") {
		t.Errorf("error should mention goreach/flush, got: %v", err)
	}
}

// errWriter is a writer that always returns an error, used to test error
// paths in WriterStorage.Store.
type errWriter struct {
	failAfter int // number of successful writes before failing
	writes    int
}

func (w *errWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.failAfter {
		return 0, fmt.Errorf("simulated write error")
	}
	return len(p), nil
}

// TestWriterStorage_Store_WriteError tests that WriterStorage.Store propagates
// errors from the underlying writer.
func TestWriterStorage_Store_WriteError(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Writer that fails on the first write (header)
	storage := WriterStorage{W: &errWriter{failAfter: 0}}
	files := []string{filepath.Join(srcDir, "covmeta.abc")}
	err := storage.Store(context.Background(), files, Metadata{})
	if err == nil {
		t.Fatal("expected error from failing writer")
	}
}

// TestWriterStorage_Store_WriteDataError tests that WriterStorage.Store
// propagates write errors when writing file data (after header succeeds).
func TestWriterStorage_Store_WriteDataError(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Writer that fails on the second write (file data, after header succeeds)
	storage := WriterStorage{W: &errWriter{failAfter: 1}}
	files := []string{filepath.Join(srcDir, "covmeta.abc")}
	err := storage.Store(context.Background(), files, Metadata{})
	if err == nil {
		t.Fatal("expected error from failing writer on data write")
	}
}

// TestWriterStorage_Store_WriteTrailingNewlineError tests that WriterStorage.Store
// propagates write errors on the trailing newline write.
func TestWriterStorage_Store_WriteTrailingNewlineError(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Writer that fails on the third write (trailing newline)
	storage := WriterStorage{W: &errWriter{failAfter: 2}}
	files := []string{filepath.Join(srcDir, "covmeta.abc")}
	err := storage.Store(context.Background(), files, Metadata{})
	if err == nil {
		t.Fatal("expected error from failing writer on trailing newline")
	}
}

// TestCopyFile_SourceNotFound tests that copyFile returns an error when
// the source file does not exist.
func TestCopyFile_SourceNotFound(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "output.txt")
	err := copyFile("/nonexistent/source.txt", dst)
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

// TestCopyFile_DestDirNotFound tests that copyFile returns an error when
// the destination directory does not exist.
func TestCopyFile_DestDirNotFound(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "source.txt")
	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join("/nonexistent/dir", "output.txt")
	err := copyFile(src, dst)
	if err == nil {
		t.Fatal("expected error for nonexistent destination directory")
	}
}

// TestCopyFile_Success tests the happy path of copyFile.
func TestCopyFile_Success(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "source.txt")
	content := "hello world"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	dst := filepath.Join(dstDir, "dest.txt")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("copied content = %q, want %q", data, content)
	}
}

// Ensure io is used (WriterStorage implements Storage using io.Writer).
var _ io.Writer = &errWriter{}
