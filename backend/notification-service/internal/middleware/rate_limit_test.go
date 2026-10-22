package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRateLimitEnforcesBurstPerClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for i := 0; i < defaultRateLimitBurst; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("request %d: got status %d, want %d", i+1, w.Code, http.StatusNoContent)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	if got := w.Body.String(); got != "rate limit exceeded" {
		t.Fatalf("got body %q, want %q", got, "rate limit exceeded")
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("got Retry-After %q, want %q", got, "1")
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.2:1234"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("different client IP: got status %d, want %d", w.Code, http.StatusNoContent)
	}
}
