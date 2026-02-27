package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
	}
	if rec.Body.Len() == 0 {
		t.Fatal("body is empty")
	}
}

func TestMakeReportHandler(t *testing.T) {
	data := []byte(`{"version":1,"packages":[]}`)
	handler := makeReportHandler(data)

	req := httptest.NewRequest(http.MethodGet, "/api/report", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatal("response body is not valid JSON")
	}
	if rec.Body.String() != string(data) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), string(data))
	}
}

func TestMakeCapabilitiesHandler(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := makeCapabilitiesHandler(tt.enabled)
			req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			var resp capabilitiesResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.SourcePreview != tt.want {
				t.Fatalf("source_preview = %v, want %v", resp.SourcePreview, tt.want)
			}
		})
	}
}

func TestReadModulePath(t *testing.T) {
	dir := t.TempDir()
	gomod := filepath.Join(dir, "go.mod")

	t.Run("valid", func(t *testing.T) {
		os.WriteFile(gomod, []byte("module github.com/example/project\n\ngo 1.21\n"), 0644)
		got, err := readModulePath(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "github.com/example/project" {
			t.Fatalf("got %q, want %q", got, "github.com/example/project")
		}
	})

	t.Run("missing", func(t *testing.T) {
		emptyDir := t.TempDir()
		_, err := readModulePath(emptyDir)
		if err == nil {
			t.Fatal("expected error for missing go.mod")
		}
	})

	t.Run("no module directive", func(t *testing.T) {
		os.WriteFile(gomod, []byte("go 1.21\n"), 0644)
		_, err := readModulePath(dir)
		if err == nil {
			t.Fatal("expected error for missing module directive")
		}
	})
}

func TestBuildFileWhitelist(t *testing.T) {
	data := []byte(`{
		"packages": [
			{
				"files": [
					{"file_name": "github.com/ex/proj/main.go"},
					{"file_name": "github.com/ex/proj/util.go"}
				]
			},
			{
				"files": [
					{"file_name": "github.com/ex/proj/pkg/handler.go"}
				]
			}
		]
	}`)
	wl, err := buildFileWhitelist(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wl) != 3 {
		t.Fatalf("len = %d, want 3", len(wl))
	}
	if !wl["github.com/ex/proj/main.go"] {
		t.Fatal("main.go not in whitelist")
	}
	if !wl["github.com/ex/proj/pkg/handler.go"] {
		t.Fatal("handler.go not in whitelist")
	}
}

func TestResolveSourcePath(t *testing.T) {
	srcDir := t.TempDir()
	// Create a file to resolve
	subDir := filepath.Join(srcDir, "internal", "pkg")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "foo.go"), []byte("package pkg\n"), 0644)

	t.Run("valid", func(t *testing.T) {
		got, err := resolveSourcePath("github.com/ex/proj/internal/pkg/foo.go", "github.com/ex/proj", srcDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// EvalSymlinks may resolve the temp dir, so check suffix
		if !filepath.IsAbs(got) {
			t.Fatalf("got relative path: %s", got)
		}
		if !strings.HasSuffix(got, filepath.Join("internal", "pkg", "foo.go")) {
			t.Fatalf("got %q, want suffix internal/pkg/foo.go", got)
		}
	})

	t.Run("not in module", func(t *testing.T) {
		_, err := resolveSourcePath("github.com/other/pkg/foo.go", "github.com/ex/proj", srcDir)
		if err == nil {
			t.Fatal("expected error for file not in module")
		}
	})

	t.Run("path traversal", func(t *testing.T) {
		_, err := resolveSourcePath("github.com/ex/proj/../../etc/passwd", "github.com/ex/proj", srcDir)
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
	})
}

func TestMakeSourceHandler_Success(t *testing.T) {
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "internal"), 0755)
	os.WriteFile(filepath.Join(srcDir, "internal", "foo.go"), []byte("package internal\nfunc Foo() {\n\ta := 1\n\tb := 2\n\tc := 3\n\td := 4\n\te := 5\n\tf := 6\n\treturn\n}\n"), 0644)

	reportData := []byte(`{
		"packages": [{
			"files": [{
				"file_name": "github.com/ex/proj/internal/foo.go",
				"functions": [{
					"unreached_blocks": [{"start_line": 4, "end_line": 5}]
				}]
			}]
		}]
	}`)

	whitelist := map[string]bool{"github.com/ex/proj/internal/foo.go": true}
	handler := makeSourceHandler("github.com/ex/proj", srcDir, whitelist, buildUnreachedMap(reportData))

	req := httptest.NewRequest(http.MethodGet, "/api/source?file=github.com/ex/proj/internal/foo.go&start=4&end=5", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Lines) == 0 {
		t.Fatal("expected lines in response")
	}

	// Check that unreached lines are marked
	foundUnreached := false
	for _, l := range resp.Lines {
		if l.Number == 4 && l.Unreached {
			foundUnreached = true
		}
	}
	if !foundUnreached {
		t.Fatal("expected line 4 to be marked as unreached")
	}
}

func TestMakeSourceHandler_NotInWhitelist(t *testing.T) {
	whitelist := map[string]bool{"github.com/ex/proj/allowed.go": true}
	handler := makeSourceHandler("github.com/ex/proj", t.TempDir(), whitelist, buildUnreachedMap([]byte(`{"packages":[]}`)))

	req := httptest.NewRequest(http.MethodGet, "/api/source?file=github.com/ex/proj/secret.go&start=1&end=5", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestMakeSourceHandler_PathTraversal(t *testing.T) {
	srcDir := t.TempDir()
	whitelist := map[string]bool{"github.com/ex/proj/../../etc/passwd": true}
	handler := makeSourceHandler("github.com/ex/proj", srcDir, whitelist, buildUnreachedMap([]byte(`{"packages":[]}`)))

	req := httptest.NewRequest(http.MethodGet, "/api/source?file=github.com/ex/proj/../../etc/passwd&start=1&end=5", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("expected non-200 status for path traversal")
	}
}

func TestMakeSourceHandler_MissingParams(t *testing.T) {
	handler := makeSourceHandler("github.com/ex/proj", t.TempDir(), map[string]bool{}, buildUnreachedMap([]byte(`{"packages":[]}`)))

	tests := []struct {
		name string
		url  string
	}{
		{"no file", "/api/source?start=1&end=5"},
		{"no start", "/api/source?file=x&end=5"},
		{"no end", "/api/source?file=x&start=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}
