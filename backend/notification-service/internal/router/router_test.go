package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"notification-service/internal/handlers"

	"github.com/gin-gonic/gin"
)

func TestNewAppliesRateLimitToNotificationAPIOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(handlers.NewNotificationHandler(nil), "test-secret", nil)

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

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health request: got status %d, want %d", w.Code, http.StatusOK)
	}
}
