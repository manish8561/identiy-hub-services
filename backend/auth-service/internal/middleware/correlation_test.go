package middleware

import (
	"net/http"
	"net/http/httptest"
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
	if !validCorrelationID(id) || id == "not a valid id" {
		t.Fatalf("response correlation ID = %q, want generated valid ID", id)
	}
}
