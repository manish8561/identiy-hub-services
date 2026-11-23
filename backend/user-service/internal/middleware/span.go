package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	SpanIDHeader       = "X-Span-ID"
	ParentSpanIDHeader = "X-Parent-Span-ID"
)

func Span() gin.HandlerFunc {
	return func(c *gin.Context) {
		parentSpanID := c.GetHeader(SpanIDHeader)
		if !validSpanID(parentSpanID) {
			parentSpanID = ""
		} else {
			parentSpanID = strings.ToLower(parentSpanID)
		}

		id, err := uuid.NewRandom()
		if err != nil {
			c.AbortWithStatusJSON(500, gin.H{"error": "failed to create span ID"})
			return
		}
		spanID := strings.ReplaceAll(id.String(), "-", "")[:16]
		c.Request.Header.Set(SpanIDHeader, spanID)
		c.Request.Header.Del(ParentSpanIDHeader)
		c.Writer.Header().Set(SpanIDHeader, spanID)
		c.Writer.Header().Del(ParentSpanIDHeader)
		if parentSpanID != "" {
			c.Request.Header.Set(ParentSpanIDHeader, parentSpanID)
			c.Writer.Header().Set(ParentSpanIDHeader, parentSpanID)
		}
		c.Set("span_id", spanID)
		c.Set("parent_span_id", parentSpanID)
		c.Next()
	}
}

func validSpanID(id string) bool {
	if len(id) != 16 {
		return false
	}
	for _, char := range id {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}
