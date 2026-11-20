package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

const CorrelationIDHeader = "X-Correlation-ID"

func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(CorrelationIDHeader)
		if !validCorrelationID(id) {
			var err error
			id, err = newCorrelationID()
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to create correlation ID"})
				return
			}
		}

		c.Request.Header.Set(CorrelationIDHeader, id)
		c.Writer.Header().Set(CorrelationIDHeader, id)
		c.Set("correlation_id", id)
		c.Next()
	}
}

func validCorrelationID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == ':' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func newCorrelationID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(id[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
