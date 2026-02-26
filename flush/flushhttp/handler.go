// Package flushhttp provides an HTTP handler for triggering coverage flush.
//
// Import this package only when you need HTTP-based coverage control.
// If you only need timer/signal-based flush, use the flush package directly.
package flushhttp

import (
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
// Endpoints:
//
//	GET  /internal/coverage       — returns current coverage data as text profile
//	POST /internal/coverage/flush — flushes to Storage, then returns status
//	POST /internal/coverage/clear — resets coverage counters (atomic mode only)
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/coverage", handleGet)
	mux.HandleFunc("POST /internal/coverage/flush", handleFlush)
	mux.HandleFunc("POST /internal/coverage/clear", handleClear)
	return http.StripPrefix("", mux)
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := coverage.WriteCounters(w); err != nil {
		http.Error(w, fmt.Sprintf("goreach: write counters: %v", err), http.StatusInternalServerError)
		return
	}
}

func handleFlush(w http.ResponseWriter, r *http.Request) {
	if err := flush.Flush(); err != nil {
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
