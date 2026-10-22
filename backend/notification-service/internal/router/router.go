package router

import (
	"net/http"

	"notification-service/internal/handlers"
	"notification-service/internal/middleware"

	"github.com/gin-gonic/gin"
)

func New(handler *handlers.NotificationHandler, secret string, origins []string) *gin.Engine {
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		for _, allowed := range origins {
			if origin == allowed {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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
