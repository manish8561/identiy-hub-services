package router

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestWithTraceIDPreservesValidID(t *testing.T) {
	handler := withCorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(traceIDHeader); got != "0123456789abcdef0123456789abcdef" {
			t.Errorf("request trace ID = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(traceIDHeader, "0123456789abcdef0123456789abcdef")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get(traceIDHeader); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("response trace ID = %q", got)
	}
}

func TestWithTraceIDGeneratesValidID(t *testing.T) {
	handler := withCorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	id := w.Header().Get(traceIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("response trace ID = %q, want 32-character hex UUID trace ID", id)
	}
}
