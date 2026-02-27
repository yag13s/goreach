package objstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yag13s/goreach/flush"
)

// uploadCall records the arguments passed to a mock Uploader.
type uploadCall struct {
	Key  string
	Body []byte
}

// mockUploader returns an Uploader that records calls for later inspection.
// If err is non-nil, the uploader returns it immediately.
func mockUploader(calls *[]uploadCall, err error) Uploader {
	return func(_ context.Context, key string, body io.Reader) error {
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(body)
		if readErr != nil {
			return readErr
		}
		*calls = append(*calls, uploadCall{Key: key, Body: data})
		return nil
	}
}

func TestStorage_Store(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("meta-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "covcounters.abc"), []byte("counter-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls []uploadCall
	storage := &Storage{Upload: mockUploader(&calls, nil)}

	files := []string{
		filepath.Join(srcDir, "covmeta.abc"),
		filepath.Join(srcDir, "covcounters.abc"),
	}
	meta := flush.Metadata{
		ServiceName:  "test-svc",
		BuildVersion: "abc123",
		PodName:      "pod-0",
	}

	if err := storage.Store(context.Background(), files, meta); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 2 {
		t.Fatalf("Upload called %d times, want 2", len(calls))
	}

	// First file
	wantKey := "goreach/test-svc/abc123/pod-0/covmeta.abc"
	if calls[0].Key != wantKey {
		t.Errorf("calls[0].Key = %q, want %q", calls[0].Key, wantKey)
	}
	if string(calls[0].Body) != "meta-data" {
		t.Errorf("calls[0].Body = %q, want %q", calls[0].Body, "meta-data")
	}

	// Second file
	wantKey = "goreach/test-svc/abc123/pod-0/covcounters.abc"
	if calls[1].Key != wantKey {
		t.Errorf("calls[1].Key = %q, want %q", calls[1].Key, wantKey)
	}
	if string(calls[1].Body) != "counter-data" {
		t.Errorf("calls[1].Body = %q, want %q", calls[1].Body, "counter-data")
	}
}

func TestStorage_Store_CustomPrefix(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls []uploadCall
	storage := &Storage{
		Upload: mockUploader(&calls, nil),
		Prefix: "custom/prefix",
	}

	files := []string{filepath.Join(srcDir, "covmeta.abc")}
	meta := flush.Metadata{
		ServiceName:  "svc",
		BuildVersion: "v1",
		PodName:      "pod",
	}

	if err := storage.Store(context.Background(), files, meta); err != nil {
		t.Fatal(err)
	}

	wantKey := "custom/prefix/svc/v1/pod/covmeta.abc"
	if calls[0].Key != wantKey {
		t.Errorf("Key = %q, want %q", calls[0].Key, wantKey)
	}
}

func TestStorage_Store_CustomKeyFunc(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls []uploadCall
	storage := &Storage{
		Upload: mockUploader(&calls, nil),
		KeyFunc: func(prefix string, meta flush.Metadata, filename string) string {
			return fmt.Sprintf("flat/%s", filename)
		},
	}

	files := []string{filepath.Join(srcDir, "covmeta.abc")}

	if err := storage.Store(context.Background(), files, flush.Metadata{}); err != nil {
		t.Fatal(err)
	}

	wantKey := "flat/covmeta.abc"
	if calls[0].Key != wantKey {
		t.Errorf("Key = %q, want %q", calls[0].Key, wantKey)
	}
}

func TestStorage_Store_EmptyFiles(t *testing.T) {
	var calls []uploadCall
	storage := &Storage{Upload: mockUploader(&calls, nil)}

	if err := storage.Store(context.Background(), nil, flush.Metadata{}); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 0 {
		t.Errorf("Upload called %d times, want 0", len(calls))
	}
}

func TestStorage_Store_FileNotFound(t *testing.T) {
	var calls []uploadCall
	storage := &Storage{Upload: mockUploader(&calls, nil)}

	files := []string{"/nonexistent/path/covmeta.abc"}
	err := storage.Store(context.Background(), files, flush.Metadata{})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "goreach/flush") {
		t.Errorf("error should mention goreach/flush, got: %v", err)
	}
}

func TestStorage_Store_UploadError(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "covmeta.abc"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls []uploadCall
	storage := &Storage{Upload: mockUploader(&calls, fmt.Errorf("access denied"))}

	files := []string{filepath.Join(srcDir, "covmeta.abc")}
	err := storage.Store(context.Background(), files, flush.Metadata{})
	if err == nil {
		t.Fatal("expected error from Upload")
	}
	if !strings.Contains(err.Error(), "goreach/flush") {
		t.Errorf("error should mention goreach/flush, got: %v", err)
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error should wrap original, got: %v", err)
	}
}

func TestDefaultKey(t *testing.T) {
	meta := flush.Metadata{
		ServiceName:  "my-svc",
		BuildVersion: "v2.0.1",
		PodName:      "my-svc-abc-xyz",
	}

	got := defaultKey("goreach", meta, "covmeta.12345")
	want := "goreach/my-svc/v2.0.1/my-svc-abc-xyz/covmeta.12345"
	if got != want {
		t.Errorf("defaultKey() = %q, want %q", got, want)
	}
}
