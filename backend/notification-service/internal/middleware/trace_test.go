package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTraceIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceID())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	preserved := httptest.NewRequest(http.MethodGet, "/", nil)
	preserved.Header.Set(TraceIDHeader, "0123456789abcdef0123456789abcdef")
	preservedResponse := httptest.NewRecorder()
	router.ServeHTTP(preservedResponse, preserved)
	if got := preservedResponse.Header().Get(TraceIDHeader); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("preserved trace ID = %q", got)
	}

	generated := httptest.NewRequest(http.MethodGet, "/", nil)
	generatedResponse := httptest.NewRecorder()
	router.ServeHTTP(generatedResponse, generated)
	id := generatedResponse.Header().Get(TraceIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("generated trace ID = %q", id)
	}
}
