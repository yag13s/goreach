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

	"github.com/yag13s/goreach/internal/report"
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

	// The report is served to the browser byte-for-byte as it was read;
	// decoding it here validates it and feeds the source preview.
	var rpt report.Report
	if err := json.Unmarshal(data, &rpt); err != nil {
		return fmt.Errorf("invalid report JSON in %s: %w", reportPath, err)
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
		whitelist, unreachedMap, latestUnreachedMap := buildSourceMaps(&rpt)
		mux.Handle("GET /api/capabilities", makeCapabilitiesHandler(true))
		mux.Handle("GET /api/source", makeSourceHandler(modulePath, opts.SrcDir, whitelist, unreachedMap, latestUnreachedMap))
	} else {
		mux.Handle("GET /api/capabilities", makeCapabilitiesHandler(false))
	}

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

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
		_ = srv.Shutdown(shutCtx)
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
		if modulePath, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(modulePath), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan go.mod: %w", err)
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

// buildSourceMaps extracts the file whitelist, unreached line map, and latest
// unreached line map for source preview from the report.
func buildSourceMaps(rpt *report.Report) (whitelist map[string]bool, unreachedMap, latestUnreachedMap map[string]map[int]bool) {
	whitelist = make(map[string]bool)
	unreachedMap = make(map[string]map[int]bool)
	latestUnreachedMap = make(map[string]map[int]bool)

	for _, pkg := range rpt.Packages {
		for _, f := range pkg.Files {
			if f.FileName != "" {
				whitelist[f.FileName] = true
			}
			for _, fn := range f.Functions {
				// Unreached blocks (skip when latest exists — old-build line numbers don't map to current source)
				if len(fn.LatestUnreachedBlocks) == 0 {
					markLines(unreachedMap, f.FileName, fn.UnreachedBlocks)
				}
				markLines(latestUnreachedMap, f.FileName, fn.LatestUnreachedBlocks)
			}
		}
	}
	return whitelist, unreachedMap, latestUnreachedMap
}

// markLines records every line spanned by blocks in lines[fileName].
func markLines(lines map[string]map[int]bool, fileName string, blocks []report.UnreachedBlock) {
	if len(blocks) == 0 {
		return
	}
	if lines[fileName] == nil {
		lines[fileName] = make(map[int]bool)
	}
	for _, b := range blocks {
		for l := b.StartLine; l <= b.EndLine; l++ {
			lines[fileName][l] = true
		}
	}
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

type capabilitiesResponse struct {
	SourcePreview bool `json:"source_preview"`
}

type sourceLine struct {
	Number          int    `json:"number"`
	Text            string `json:"text"`
	Unreached       bool   `json:"unreached"`
	LatestUnreached bool   `json:"latest_unreached,omitempty"`
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

func makeSourceHandler(modulePath, srcDir string, whitelist map[string]bool, unreachedMap, latestUnreachedMap map[string]map[int]bool) http.Handler {
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
		latestUnreachedLines := latestUnreachedMap[fileName]

		// Add 3 lines of context before and after
		contextStart := max(start-3, 1)
		contextEnd := min(end+3, len(lines))

		var result []sourceLine
		for i := contextStart; i <= contextEnd; i++ {
			result = append(result, sourceLine{
				Number:          i,
				Text:            lines[i-1],
				Unreached:       unreachedLines[i],
				LatestUnreached: latestUnreachedLines[i],
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
	_ = cmd.Start()
}
