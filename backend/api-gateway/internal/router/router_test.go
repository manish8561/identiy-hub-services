package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiter_DefaultValues(t *testing.T) {
	limiter, rps := NewRateLimiter()

	assert.NotNil(t, limiter)
	assert.Equal(t, 10.0, rps)
}

func TestNewRateLimiter_CustomValues(t *testing.T) {
	os.Setenv("RATE_LIMIT_RPS", "20")
	os.Setenv("RATE_LIMIT_BURST", "40")
	defer os.Unsetenv("RATE_LIMIT_RPS")
	defer os.Unsetenv("RATE_LIMIT_BURST")

	limiter, rps := NewRateLimiter()

	assert.NotNil(t, limiter)
	assert.Equal(t, 20.0, rps)
}

func TestRateLimiter_Allow(t *testing.T) {
	limiter := newLimiter(10.0, 20)

	// Should allow requests within burst limit
	for i := 0; i < 20; i++ {
		assert.True(t, limiter.allow("127.0.0.1"))
	}

	// Should deny after burst is exhausted
	assert.False(t, limiter.allow("127.0.0.1"))
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	limiter := newLimiter(10.0, 20)

	// Different IPs should have separate buckets
	for i := 0; i < 20; i++ {
		assert.True(t, limiter.allow("127.0.0.1"))
		assert.True(t, limiter.allow("192.168.1.1"))
	}
}

func TestRateLimiter_ZeroRateLimit(t *testing.T) {
	limiter := newLimiter(0, 0)

	// Should always allow when rate limit is disabled
	for i := 0; i < 100; i++ {
		assert.True(t, limiter.allow("127.0.0.1"))
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")

	ip := clientIP(req)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")

	ip := clientIP(req)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	ip := clientIP(req)
	assert.Equal(t, "127.0.0.1", ip)
}

func TestClientIP_UsesRemoteAddrWhenHeadersMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.7:4321"
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Real-IP")

	assert.Equal(t, "10.0.0.7", clientIP(req))
}

func TestWriteRateLimitResponse(t *testing.T) {
	w := httptest.NewRecorder()

	writeRateLimitResponse(w, 5.0)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "1", w.Header().Get("Retry-After"))
	assert.Equal(t, "rate limit exceeded", w.Body.String())

	w = httptest.NewRecorder()
	writeRateLimitResponse(w, 0)
	assert.Empty(t, w.Header().Get("Retry-After"))
}

func TestRegisterHealthEndpoints(t *testing.T) {
	srv := kratoshttp.NewServer()
	RegisterHealthEndpoints(srv)

	// We can't easily test the registered handlers without starting the server
	// This test mainly ensures the function doesn't panic
	assert.NotNil(t, srv)
}

func TestRegisterSwaggerEndpoints(t *testing.T) {
	srv := kratoshttp.NewServer()
	RegisterSwaggerEndpoints(srv)

	// We can't easily test the registered handlers without starting the server
	// This test mainly ensures the function doesn't panic
	assert.NotNil(t, srv)
}

func TestRegisterProxyRoutes(t *testing.T) {
	limiter, rps := NewRateLimiter()
	srv := kratoshttp.NewServer()
	RegisterProxyRoutes(srv, limiter, rps)

	// We can't easily test the registered handlers without starting the server
	// This test mainly ensures the function doesn't panic
	assert.NotNil(t, srv)
}

func TestUpstreamHTTPClientHasTimeout(t *testing.T) {
	assert.Equal(t, 10*time.Second, upstreamHTTPClient.Timeout)
}

func TestMinFloat(t *testing.T) {
	assert.Equal(t, 1.0, minFloat(1.0, 2.0))
	assert.Equal(t, 1.0, minFloat(2.0, 1.0))
	assert.Equal(t, 1.5, minFloat(1.5, 1.5))
}
