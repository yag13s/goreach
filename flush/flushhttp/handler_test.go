package flushhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_Routing(t *testing.T) {
	h := Handler()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "GET coverage",
			method:     "GET",
			path:       "/internal/coverage",
			wantStatus: http.StatusInternalServerError, // no -cover build
		},
		{
			name:       "POST flush",
			method:     "POST",
			path:       "/internal/coverage/flush",
			wantStatus: http.StatusOK, // Emit() returns nil when not enabled
		},
		{
			name:       "POST clear",
			method:     "POST",
			path:       "/internal/coverage/clear",
			wantStatus: http.StatusInternalServerError, // no -cover build
		},
		{
			name:       "wrong method for GET endpoint",
			method:     "POST",
			path:       "/internal/coverage",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "wrong method for flush endpoint",
			method:     "GET",
			path:       "/internal/coverage/flush",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "unknown path",
			method:     "GET",
			path:       "/internal/coverage/unknown",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHandleFlush_ResponseJSON(t *testing.T) {
	h := Handler()

	req := httptest.NewRequest("POST", "/internal/coverage/flush", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
	if resp["flushed"] == "" {
		t.Error("flushed timestamp should not be empty")
	}
}
