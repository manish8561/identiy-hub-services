package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultRateLimitRPS   = 10.0
	defaultRateLimitBurst = 20
	maxRateLimitBuckets   = 10000
)

type rateLimitBucket struct {
	tokens   float64
	lastUsed time.Time
}

type rateLimiter struct {
	rps     float64
	burst   int
	mu      sync.Mutex
	buckets map[string]*rateLimitBucket
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	return &rateLimiter{
		rps:     rps,
		burst:   burst,
		buckets: make(map[string]*rateLimitBucket),
	}
}

func (l *rateLimiter) allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.buckets[ip]
	if !exists {
		if len(l.buckets) >= maxRateLimitBuckets {
			l.evictOldest()
		}
		l.buckets[ip] = &rateLimitBucket{tokens: float64(l.burst - 1), lastUsed: now}
		return true
	}

	elapsed := now.Sub(bucket.lastUsed).Seconds()
	bucket.tokens = math.Min(float64(l.burst), bucket.tokens+elapsed*l.rps)
	bucket.lastUsed = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (l *rateLimiter) evictOldest() {
	var oldestIP string
	var oldest time.Time
	for ip, bucket := range l.buckets {
		if oldestIP == "" || bucket.lastUsed.Before(oldest) {
			oldestIP = ip
			oldest = bucket.lastUsed
		}
	}
	if oldestIP != "" {
		delete(l.buckets, oldestIP)
	}
}

// RateLimit applies a per-client-IP token bucket to service API requests.
func RateLimit() gin.HandlerFunc {
	limiter := newRateLimiter(defaultRateLimitRPS, defaultRateLimitBurst)
	return func(c *gin.Context) {
		if !limiter.allow(clientIP(c.Request)) {
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.Header("Retry-After", strconv.Itoa(int(math.Ceil(1/defaultRateLimitRPS))))
			c.String(http.StatusTooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}
		c.Next()
	}
}

func clientIP(r *http.Request) string {
	for _, forwarded := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if ip := strings.TrimSpace(forwarded); ip != "" {
			return ip
		}
	}
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
