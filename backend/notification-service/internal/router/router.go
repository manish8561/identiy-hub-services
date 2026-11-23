package router

import (
	"net/http"

	"notification-service/internal/handlers"
	"notification-service/internal/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func New(handler *handlers.NotificationHandler, secret string, origins []string, log *zap.Logger) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CorrelationID())
	r.Use(middleware.TraceID())
	r.Use(middleware.Span())
	r.Use(middleware.RequestLogging(log))
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		for _, allowed := range origins {
			if origin == allowed {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Correlation-ID, X-Trace-ID, X-Span-ID")
				c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Correlation-ID, X-Trace-ID, X-Span-ID, X-Parent-Span-ID")
				c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
				break
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})
	api := r.Group("/api/notifications", middleware.RateLimit(), middleware.CookieAuth(secret))
	api.GET("/preferences", handler.GetPreference)
	api.PUT("/preferences", handler.UpdatePreference)
	api.GET("/history", handler.History)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "notification-service"})
	})
	return r
}
