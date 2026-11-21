package flushhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
			wantStatus: http.StatusServiceUnavailable, // flush.Enable not called
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

// The success path needs a -cover binary; see TestIntegration_HTTPHandler in
// package flush.
func TestHandleFlush_NotEnabled(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/internal/coverage/flush", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "not enabled") {
		t.Errorf("body = %q, want it to say flushing is not enabled", rec.Body)
	}
}
