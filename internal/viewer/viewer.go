// Package viewer serves a web UI for goreach report.json files.
package viewer

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed index.html
var indexHTML []byte

// Options configures the viewer server.
type Options struct {
	Port   int    // 0 = random available port
	NoOpen bool   // do not auto-open browser
	SrcDir string // source root for code preview (empty = disabled)
}

// Serve starts an HTTP server that serves the report viewer UI.
// It blocks until SIGINT/SIGTERM is received.
func Serve(reportPath string, opts Options) error {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}

	// Validate JSON
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON in %s", reportPath)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.Handle("GET /api/report", makeReportHandler(data))

	if opts.SrcDir != "" {
		modulePath, err := readModulePath(opts.SrcDir)
		if err != nil {
			return fmt.Errorf("read module path: %w", err)
		}
		whitelist, err := buildFileWhitelist(data)
		if err != nil {
			return fmt.Errorf("build file whitelist: %w", err)
		}
		unreachedMap := buildUnreachedMap(data)
		mux.Handle("GET /api/capabilities", makeCapabilitiesHandler(true))
		mux.Handle("GET /api/source", makeSourceHandler(modulePath, opts.SrcDir, whitelist, unreachedMap))
	} else {
		mux.Handle("GET /api/capabilities", makeCapabilitiesHandler(false))
	}

	srv := &http.Server{Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	url := fmt.Sprintf("http://%s", ln.Addr().String())
	fmt.Fprintf(os.Stderr, "goreach view: serving at %s\n", url)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n")

	if !opts.NoOpen {
		openBrowser(url)
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func makeReportHandler(data []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
}

// readModulePath reads go.mod in srcDir and returns the module path.
func readModulePath(srcDir string) (string, error) {
	f, err := os.Open(filepath.Join(srcDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan go.mod: %w", err)
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

// buildFileWhitelist extracts all file_name values from the report JSON.
func buildFileWhitelist(data []byte) (map[string]bool, error) {
	var rpt struct {
		Packages []struct {
			Files []struct {
				FileName string `json:"file_name"`
			} `json:"files"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &rpt); err != nil {
		return nil, fmt.Errorf("parse report for whitelist: %w", err)
	}
	wl := make(map[string]bool)
	for _, pkg := range rpt.Packages {
		for _, f := range pkg.Files {
			if f.FileName != "" {
				wl[f.FileName] = true
			}
		}
	}
	return wl, nil
}

// resolveSourcePath converts a report file_name (import path form) to an
// absolute path under srcDir, validating that it stays within the source root.
func resolveSourcePath(fileName, modulePath, srcDir string) (string, error) {
	rel := strings.TrimPrefix(fileName, modulePath)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == fileName {
		return "", fmt.Errorf("file %q does not belong to module %q", fileName, modulePath)
	}

	joined := filepath.Join(srcDir, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	absSrc, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		return "", fmt.Errorf("resolve srcDir: %w", err)
	}

	if !strings.HasPrefix(resolved, absSrc+string(filepath.Separator)) && resolved != absSrc {
		return "", fmt.Errorf("path %q is outside source root", fileName)
	}
	return resolved, nil
}

// readLines reads all lines from a file.
func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	// Remove trailing empty line from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

// buildUnreachedMap precomputes a map of file_name -> set of unreached line numbers
// from the report JSON. This avoids re-parsing JSON on every /api/source request.
func buildUnreachedMap(data []byte) map[string]map[int]bool {
	var rpt struct {
		Packages []struct {
			Files []struct {
				FileName  string `json:"file_name"`
				Functions []struct {
					UnreachedBlocks []struct {
						StartLine int `json:"start_line"`
						EndLine   int `json:"end_line"`
					} `json:"unreached_blocks"`
				} `json:"functions"`
			} `json:"files"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &rpt); err != nil {
		return nil
	}
	result := make(map[string]map[int]bool)
	for _, pkg := range rpt.Packages {
		for _, f := range pkg.Files {
			for _, fn := range f.Functions {
				for _, b := range fn.UnreachedBlocks {
					if result[f.FileName] == nil {
						result[f.FileName] = make(map[int]bool)
					}
					for l := b.StartLine; l <= b.EndLine; l++ {
						result[f.FileName][l] = true
					}
				}
			}
		}
	}
	return result
}

type capabilitiesResponse struct {
	SourcePreview bool `json:"source_preview"`
}

type sourceLine struct {
	Number    int    `json:"number"`
	Text      string `json:"text"`
	Unreached bool   `json:"unreached"`
}

type sourceResponse struct {
	Lines []sourceLine `json:"lines"`
}

func makeCapabilitiesHandler(sourceEnabled bool) http.Handler {
	resp, _ := json.Marshal(capabilitiesResponse{SourcePreview: sourceEnabled})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	})
}

func makeSourceHandler(modulePath, srcDir string, whitelist map[string]bool, unreachedMap map[string]map[int]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileName := r.URL.Query().Get("file")
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		if fileName == "" || startStr == "" || endStr == "" {
			http.Error(w, "missing file, start, or end parameter", http.StatusBadRequest)
			return
		}

		start, err := strconv.Atoi(startStr)
		if err != nil || start < 1 {
			http.Error(w, "invalid start parameter", http.StatusBadRequest)
			return
		}
		end, err := strconv.Atoi(endStr)
		if err != nil || end < start {
			http.Error(w, "invalid end parameter", http.StatusBadRequest)
			return
		}

		if !whitelist[fileName] {
			http.Error(w, "file not in report", http.StatusForbidden)
			return
		}

		resolved, err := resolveSourcePath(fileName, modulePath, srcDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		lines, err := readLines(resolved)
		if err != nil {
			http.Error(w, "read source file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		unreachedLines := unreachedMap[fileName]

		// Add 3 lines of context before and after
		contextStart := start - 3
		if contextStart < 1 {
			contextStart = 1
		}
		contextEnd := end + 3
		if contextEnd > len(lines) {
			contextEnd = len(lines)
		}

		var result []sourceLine
		for i := contextStart; i <= contextEnd; i++ {
			result = append(result, sourceLine{
				Number:    i,
				Text:      lines[i-1],
				Unreached: unreachedLines[i],
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sourceResponse{Lines: result})
	})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	cmd.Start()
}
