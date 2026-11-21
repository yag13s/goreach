// Package flushhttp provides an HTTP handler for triggering coverage flush.
//
// Import this package only when you need HTTP-based coverage control.
// If you only need timer/signal-based flush, use the flush package directly.
package flushhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/coverage"
	"time"

	"github.com/yag13s/goreach/flush"
)

// Handler returns an http.Handler that exposes coverage data endpoints.
//
// The endpoints are unauthenticated, and /flush and /clear change state.
// Serve them only on an internal listener, or wrap the handler with your own
// authentication; never expose them to the public internet.
//
// Endpoints:
//
//	GET  /internal/coverage       — returns current coverage counters as binary data
//	POST /internal/coverage/flush — flushes to Storage, then returns status
//	                                (503 if flush.Enable has not been called)
//	POST /internal/coverage/clear — resets coverage counters (atomic mode only)
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/coverage", handleGet)
	mux.HandleFunc("POST /internal/coverage/flush", handleFlush)
	mux.HandleFunc("POST /internal/coverage/clear", handleClear)
	return mux
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	// Buffer first so a failure can still be reported as a 500 instead of a
	// 200 with a truncated body.
	var buf bytes.Buffer
	if err := coverage.WriteCounters(&buf); err != nil {
		http.Error(w, fmt.Sprintf("goreach: write counters: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(buf.Bytes())
}

func handleFlush(w http.ResponseWriter, r *http.Request) {
	if !flush.Enabled() {
		http.Error(w, "goreach: flush is not enabled (call flush.Enable in a binary built with -cover)", http.StatusServiceUnavailable)
		return
	}
	// Emit rather than EmitContext(r.Context()): the flush is bounded by
	// Config.FlushTimeout, and a client that hangs up should not abort an
	// upload that is already under way.
	if err := flush.Emit(); err != nil {
		http.Error(w, fmt.Sprintf("goreach: flush failed: %v", err), http.StatusInternalServerError)
		return
	}

	hostname, _ := os.Hostname()
	resp := map[string]string{
		"status":   "ok",
		"flushed":  time.Now().UTC().Format(time.RFC3339),
		"hostname": hostname,
		"pod_name": os.Getenv("POD_NAME"),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	if err := coverage.ClearCounters(); err != nil {
		http.Error(w, fmt.Sprintf("goreach: clear failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
