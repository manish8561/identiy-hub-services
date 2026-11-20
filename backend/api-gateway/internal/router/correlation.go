package router

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	kratoslog "github.com/go-kratos/kratos/v2/log"
)

const correlationIDHeader = "X-Correlation-ID"

// CorrelationID applies correlation ID validation, propagation, and request logging.
func CorrelationID(logger kratoslog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return withCorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if logger != nil {
				_ = logger.Log(
					kratoslog.LevelInfo,
					"msg", "HTTP request",
					"correlation_id", r.Header.Get(correlationIDHeader),
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

		r.Header.Set(correlationIDHeader, id)
		w.Header().Set(correlationIDHeader, id)
		next.ServeHTTP(w, r)
	})
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
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}
