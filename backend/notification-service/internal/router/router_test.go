package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"notification-service/internal/handlers"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestNewAppliesRateLimitToNotificationAPIOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(handlers.NewNotificationHandler(nil), "test-secret", nil, zap.NewNop())

	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/notifications/preferences", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: got status %d, want %d", i+1, w.Code, http.StatusUnauthorized)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/preferences", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("over-limit request: got status %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	if w.Header().Get("X-Correlation-ID") == "" {
		t.Fatal("over-limit response is missing X-Correlation-ID")
	}

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health request: got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestNewPropagatesCorrelationIDAndExposesItToBrowsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(
		handlers.NewNotificationHandler(nil),
		"test-secret",
		[]string{"http://localhost:3000"},
		zap.NewNop(),
	)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("X-Correlation-ID", "client-id:123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if got := w.Header().Get("X-Correlation-ID"); got != "client-id:123" {
		t.Fatalf("response correlation ID = %q, want %q", got, "client-id:123")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-Correlation-ID, X-Trace-ID" {
		t.Fatalf("allowed headers = %q", got)
	}
	if got := w.Header().Get("Access-Control-Expose-Headers"); got != "X-Correlation-ID, X-Trace-ID" {
		t.Fatalf("exposed headers = %q", got)
	}
}
