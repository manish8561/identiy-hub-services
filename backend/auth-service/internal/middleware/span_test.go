package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSpanCreatesChildSpan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Span())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(SpanIDHeader, "0123456789ABCDEF")
	req.Header.Set(ParentSpanIDHeader, "untrusted-parent")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	spanID := w.Header().Get(SpanIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(spanID) || spanID == "0123456789abcdef" {
		t.Fatalf("span ID = %q, want a newly generated 16-character hex ID", spanID)
	}
	if got := w.Header().Get(ParentSpanIDHeader); got != "0123456789abcdef" {
		t.Fatalf("parent span ID = %q", got)
	}
}

func TestSpanCreatesRootSpanWithoutValidParent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Span())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(SpanIDHeader, "invalid")
	req.Header.Set(ParentSpanIDHeader, "forged")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(w.Header().Get(SpanIDHeader)) {
		t.Fatalf("invalid generated span ID %q", w.Header().Get(SpanIDHeader))
	}
	if got := w.Header().Get(ParentSpanIDHeader); got != "" {
		t.Fatalf("root request parent span ID = %q, want empty", got)
	}
}
