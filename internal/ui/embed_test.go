package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedUI_ServeFiles(t *testing.T) {
	handler := Handler()

	tests := []struct {
		path         string
		expectedCode int
		expectedBody string
	}{
		{"/", http.StatusOK, "Sugi Observability Engine"},
		{"/style.css", http.StatusOK, "--bg-primary"},
		{"/app.js", http.StatusOK, "MiniChart"},
		{"/favicon.svg", http.StatusOK, "<svg"},
		{"/random-nonexistent-file.html", http.StatusNotFound, "Page Not Found"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedCode {
				t.Errorf("path %s: expected code %d, got %d", tt.path, tt.expectedCode, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.expectedBody) {
				t.Errorf("path %s: expected body to contain %q", tt.path, tt.expectedBody)
			}
		})
	}
}
