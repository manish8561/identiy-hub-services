package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCorrelationIDPreservesValidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CorrelationID())
	router.GET("/", func(c *gin.Context) {
		if got := c.GetHeader(CorrelationIDHeader); got != "client-id:123" {
			t.Errorf("request correlation ID = %q, want %q", got, "client-id:123")
		}
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(CorrelationIDHeader, "client-id:123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if got := w.Header().Get(CorrelationIDHeader); got != "client-id:123" {
		t.Fatalf("response correlation ID = %q, want %q", got, "client-id:123")
	}
}

func TestCorrelationIDReplacesInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CorrelationID())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(CorrelationIDHeader, "not a valid id")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	id := w.Header().Get(CorrelationIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("response correlation ID = %q, want generated UUIDv4", id)
	}
}
