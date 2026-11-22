package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const TraceIDHeader = "X-Trace-ID"

func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(TraceIDHeader)
		if !validTraceID(id) {
			id = strings.ReplaceAll(uuid.NewString(), "-", "")
		} else {
			id = strings.ToLower(id)
		}

		c.Request.Header.Set(TraceIDHeader, id)
		c.Writer.Header().Set(TraceIDHeader, id)
		c.Set("trace_id", id)
		c.Next()
	}
}

func validTraceID(id string) bool {
	if len(id) != 32 {
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
