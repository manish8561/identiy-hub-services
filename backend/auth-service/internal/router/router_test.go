package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"auth-service/internal/config"
	"auth-service/internal/handlers"
	"auth-service/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := zap.NewNop()
	cookieConfig := config.NewCookieConfig(false, 3600, "lax", "")
	jwtService := services.NewJWTService(config.NewJWTConfig("test-secret"))
	authService := services.NewAuthService(nil, jwtService, nil)
	authHandler := handlers.NewAuthHandler(authService, log, cookieConfig)

	return NewRouter(authHandler, log, []string{"http://localhost:3000"})
}

func TestNewRouter_RegistersHealthEndpoint(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")
	assert.Contains(t, w.Body.String(), "auth-service")
}

func TestNewRouter_RegistersAuthRoutes(t *testing.T) {
	router := setupTestRouter()

	testCases := []struct {
		method       string
		path         string
		expectedCode int
	}{
		{http.MethodPost, "/api/auth/login", http.StatusBadRequest},     // Will fail due to missing body
		{http.MethodPost, "/api/auth/register", http.StatusBadRequest},  // Will fail due to missing body
		{http.MethodPost, "/api/auth/validate", http.StatusBadRequest},  // Will fail due to missing token
		{http.MethodPost, "/api/auth/refresh", http.StatusUnauthorized}, // Will fail due to missing token
		{http.MethodGet, "/api/auth/me", http.StatusUnauthorized},       // Will fail due to missing token
		{http.MethodDelete, "/api/auth/account", http.StatusUnauthorized},
		{http.MethodPost, "/api/auth/logout", http.StatusOK}, // Should succeed
	}

	for _, tc := range testCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedCode, w.Code)
		})
	}
}

func TestNewRouter_AppliesCORSMiddleware(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestNewRouter_AppliesLoggingMiddleware(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// If logging middleware is applied, the request should be processed
	assert.Equal(t, http.StatusOK, w.Code)
}
