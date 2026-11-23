package middleware

import (
	"auth-service/internal/logger"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// LoggingMiddleware creates a Gin middleware for request logging using Zap
func LoggingMiddleware(log logger.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Log the request details
		log.Info("HTTP Request",
			zap.String("method", param.Method),
			zap.String("path", param.Path),
			zap.Int("status", param.StatusCode),
			zap.Duration("latency", param.Latency),
			zap.String("client_ip", param.ClientIP),
			zap.String("user_agent", param.Request.UserAgent()),
			zap.Int("body_size", param.BodySize),
			zap.String("correlation_id", param.Request.Header.Get(CorrelationIDHeader)),
			zap.String("trace_id", param.Request.Header.Get(TraceIDHeader)),
			zap.String("span_id", param.Request.Header.Get(SpanIDHeader)),
			zap.String("parent_span_id", param.Request.Header.Get(ParentSpanIDHeader)),
		)

		// Return empty string since we're using Zap for logging
		return ""
	})
}

// ZapMiddleware creates a more comprehensive Zap logging middleware
func ZapMiddleware(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Log incoming request
		log.Info("Incoming request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", raw),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.String("correlation_id", c.GetHeader(CorrelationIDHeader)),
			zap.String("trace_id", c.GetHeader(TraceIDHeader)),
			zap.String("span_id", c.GetHeader(SpanIDHeader)),
			zap.String("parent_span_id", c.GetHeader(ParentSpanIDHeader)),
		)

		// Process request
		c.Next()

		// Log response
		latency := time.Since(start)
		log.Info("Request completed",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("response_size", c.Writer.Size()),
			zap.String("correlation_id", c.GetHeader(CorrelationIDHeader)),
			zap.String("trace_id", c.GetHeader(TraceIDHeader)),
			zap.String("span_id", c.GetHeader(SpanIDHeader)),
			zap.String("parent_span_id", c.GetHeader(ParentSpanIDHeader)),
		)

		// Log errors if any
		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				log.Error("Request error",
					zap.String("method", c.Request.Method),
					zap.String("path", path),
					zap.Error(err),
					zap.String("client_ip", c.ClientIP()),
					zap.String("correlation_id", c.GetHeader(CorrelationIDHeader)),
					zap.String("trace_id", c.GetHeader(TraceIDHeader)),
					zap.String("span_id", c.GetHeader(SpanIDHeader)),
					zap.String("parent_span_id", c.GetHeader(ParentSpanIDHeader)),
				)
			}
		}
	}
}
