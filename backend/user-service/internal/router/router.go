package router

import (
	"net/http"

	"user-service/internal/handlers"
	"user-service/internal/logger"
	"user-service/internal/middleware"

	"github.com/gin-gonic/gin"
)

// NewRouter configures all routes for the user service
func NewRouter(userHandler *handlers.UserHandler, log logger.Logger, jwtSecret string, allowedOrigins []string) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.EnableCORS(allowedOrigins))
	r.Use(middleware.ZapMiddleware(log))

	api := r.Group("/api/users")
	api.Use(middleware.RateLimit())
	api.Use(middleware.CookieAuth(jwtSecret))
	{
		api.GET("/me", userHandler.GetCurrentUser)
		api.GET("/profile/:id", userHandler.GetUserByID)
		api.GET("/list", userHandler.ListUsers)
		api.PUT("/profile/:id", userHandler.UpdateUser)
		api.DELETE("/profile/:id", userHandler.DeleteUser)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "user-service"})
	})

	return r
}
