package router

import (
	"net/http"

	"auth-service/internal/handlers"
	"auth-service/internal/logger"
	"auth-service/internal/middleware"

	"github.com/gin-gonic/gin"
)

// NewRouter configures all routes for the auth service
func NewRouter(authHandler *handlers.AuthHandler, log logger.Logger, allowedOrigins []string) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.EnableCORS(allowedOrigins))
	r.Use(middleware.ZapMiddleware(log))

	api := r.Group("/api/auth")
	{
		api.POST("/login", authHandler.Login)
		api.POST("/register", authHandler.Register)
		api.POST("/validate", authHandler.ValidateToken)
		api.POST("/refresh", authHandler.RefreshToken)
		api.GET("/me", authHandler.Me)
		api.DELETE("/account", authHandler.DeleteAccount)
		api.POST("/logout", authHandler.Logout)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "auth-service"})
	})

	return r
}
