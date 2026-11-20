package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWithCorrelationIDPreservesValidID(t *testing.T) {
	handler := withCorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(correlationIDHeader); got != "client-id:123" {
			t.Errorf("request correlation ID = %q, want %q", got, "client-id:123")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(correlationIDHeader, "client-id:123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get(correlationIDHeader); got != "client-id:123" {
		t.Fatalf("response correlation ID = %q, want %q", got, "client-id:123")
	}
}

func TestWithCorrelationIDReplacesInvalidID(t *testing.T) {
	handler := withCorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validCorrelationID(r.Header.Get(correlationIDHeader)) {
			t.Errorf("request correlation ID is invalid: %q", r.Header.Get(correlationIDHeader))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(correlationIDHeader, "not a valid id")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	id := w.Header().Get(correlationIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("response correlation ID = %q, want generated UUIDv4", id)
	}
}

func TestProxyForwardsCorrelationID(t *testing.T) {
	originalClient := upstreamHTTPClient
	t.Cleanup(func() { upstreamHTTPClient = originalClient })

	var upstreamCorrelationID string
	upstreamHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			upstreamCorrelationID = req.Header.Get(correlationIDHeader)
			headers := make(http.Header)
			headers.Set(correlationIDHeader, "upstream-overrode-id")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}

	srv := kratoshttp.NewServer()
	RegisterProxyRoutes(srv, newLimiter(0, 0), 0)
	handler := withCorrelationID(srv)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set(correlationIDHeader, "client-id:123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if upstreamCorrelationID != "client-id:123" {
		t.Fatalf("upstream correlation ID = %q, want %q", upstreamCorrelationID, "client-id:123")
	}
	if got := w.Header().Get(correlationIDHeader); got != "client-id:123" {
		t.Fatalf("response correlation ID = %q, want %q", got, "client-id:123")
	}
}
