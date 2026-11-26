package router

import (
	"net/http"
	"strings"

	kratoslog "github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

const correlationIDHeader = "X-Correlation-ID"
const traceIDHeader = "X-Trace-ID"
const spanIDHeader = "X-Span-ID"
const parentSpanIDHeader = "X-Parent-Span-ID"

// CorrelationID applies correlation ID validation, propagation, and request logging.
func CorrelationID(logger kratoslog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return withCorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if logger != nil {
				_ = logger.Log(
					kratoslog.LevelInfo,
					"msg", "HTTP request",
					"correlation_id", r.Header.Get(correlationIDHeader),
					"trace_id", r.Header.Get(traceIDHeader),
					"span_id", r.Header.Get(spanIDHeader),
					"parent_span_id", r.Header.Get(parentSpanIDHeader),
					"method", r.Method,
					"path", r.URL.Path,
				)
			}
			next.ServeHTTP(w, r)
		}))
	}
}

func withCorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(correlationIDHeader)
		if !validCorrelationID(id) {
			var err error
			id, err = newCorrelationID()
			if err != nil {
				http.Error(w, "failed to create correlation ID", http.StatusInternalServerError)
				return
			}
		}

		traceID := r.Header.Get(traceIDHeader)
		if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
			traceID = sc.TraceID().String()
		} else if !validTraceID(traceID) {
			traceID = strings.ReplaceAll(uuid.NewString(), "-", "")
		} else {
			traceID = strings.ToLower(traceID)
		}

		parentSpanID := r.Header.Get(spanIDHeader)
		if !validSpanID(parentSpanID) {
			parentSpanID = ""
		} else {
			parentSpanID = strings.ToLower(parentSpanID)
		}
		spanID := ""
		if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
			spanID = sc.SpanID().String()
		} else {
			var err error
			spanID, err = newSpanID()
			if err != nil {
				http.Error(w, "failed to create span ID", http.StatusInternalServerError)
				return
			}
		}

		r.Header.Set(correlationIDHeader, id)
		r.Header.Set(traceIDHeader, traceID)
		r.Header.Set(spanIDHeader, spanID)
		r.Header.Del(parentSpanIDHeader)
		if parentSpanID != "" {
			r.Header.Set(parentSpanIDHeader, parentSpanID)
		}
		w.Header().Set(correlationIDHeader, id)
		w.Header().Set(traceIDHeader, traceID)
		w.Header().Set(spanIDHeader, spanID)
		w.Header().Del(parentSpanIDHeader)
		if parentSpanID != "" {
			w.Header().Set(parentSpanIDHeader, parentSpanID)
		}
		next.ServeHTTP(w, r)
	})
}

func validSpanID(id string) bool {
	if len(id) != 16 || id == "0000000000000000" {
		return false
	}
	for _, char := range id {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}

func newSpanID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(id.String(), "-", "")[:16], nil
}

func validTraceID(id string) bool {
	if len(id) != 32 || id == "00000000000000000000000000000000" {
		return false
	}
	for _, char := range id {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		if char >= 'A' && char <= 'F' {
			continue
		}
		return false
	}
	return true
}

func validCorrelationID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == ':' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func newCorrelationID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
