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
	return NewRouterWithAccountStatus(userHandler, log, jwtSecret, allowedOrigins, nil)
}

func NewRouterWithAccountStatus(userHandler *handlers.UserHandler, log logger.Logger, jwtSecret string, allowedOrigins []string, accountActive middleware.AccountStatusChecker) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.OpenTelemetry())
	r.Use(middleware.CorrelationID())
	r.Use(middleware.TraceID())
	r.Use(middleware.Span())
	r.Use(middleware.EnableCORS(allowedOrigins))
	r.Use(middleware.ZapMiddleware(log))

	api := r.Group("/api/users")
	api.Use(middleware.RateLimit())
	api.Use(middleware.CookieAuthWithAccountStatus(jwtSecret, accountActive))
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
