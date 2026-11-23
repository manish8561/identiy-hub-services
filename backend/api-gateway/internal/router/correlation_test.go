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
	var upstreamTraceID string
	var upstreamSpanID string
	var upstreamParentSpanID string
	upstreamHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			upstreamCorrelationID = req.Header.Get(correlationIDHeader)
			upstreamTraceID = req.Header.Get(traceIDHeader)
			upstreamSpanID = req.Header.Get(spanIDHeader)
			upstreamParentSpanID = req.Header.Get(parentSpanIDHeader)
			headers := make(http.Header)
			headers.Set(correlationIDHeader, "upstream-overrode-id")
			headers.Set(traceIDHeader, "fedcba9876543210fedcba9876543210")
			headers.Set(spanIDHeader, "fedcba9876543210")
			headers.Set(parentSpanIDHeader, "fedcba9876543210")
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
	req.Header.Set(traceIDHeader, "0123456789abcdef0123456789abcdef")
	req.Header.Set(spanIDHeader, "0123456789abcdef")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if upstreamCorrelationID != "client-id:123" {
		t.Fatalf("upstream correlation ID = %q, want %q", upstreamCorrelationID, "client-id:123")
	}
	if got := w.Header().Get(correlationIDHeader); got != "client-id:123" {
		t.Fatalf("response correlation ID = %q, want %q", got, "client-id:123")
	}
	if upstreamTraceID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("upstream trace ID = %q", upstreamTraceID)
	}
	if !validSpanID(upstreamSpanID) || upstreamSpanID == "0123456789abcdef" {
		t.Fatalf("upstream span ID = %q, want a new span ID", upstreamSpanID)
	}
	if upstreamParentSpanID != "0123456789abcdef" {
		t.Fatalf("upstream parent span ID = %q", upstreamParentSpanID)
	}
	if got := w.Header().Get(traceIDHeader); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("response trace ID = %q", got)
	}
	if got := w.Header().Get(spanIDHeader); got != upstreamSpanID {
		t.Fatalf("response span ID = %q, want gateway span %q", got, upstreamSpanID)
	}
	if got := w.Header().Get(parentSpanIDHeader); got != "0123456789abcdef" {
		t.Fatalf("response parent span ID = %q", got)
	}
}
