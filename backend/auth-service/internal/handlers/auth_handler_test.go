package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"auth-service/internal/config"
	"auth-service/internal/handlers"
	"auth-service/internal/models"
	authrouter "auth-service/internal/router"
	"auth-service/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func setupAuthRouter(t *testing.T) (*gin.Engine, *config.CookieConfig, *services.JWTService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cookieConfig := config.NewCookieConfig(false, 3600, "lax", "")
	jwtService := services.NewJWTService(config.NewJWTConfig("test-secret"))
	authService := services.NewAuthService(nil, jwtService, nil)
	handler := handlers.NewAuthHandler(authService, zap.NewNop(), cookieConfig)

	r := authrouter.NewRouter(handler, zap.NewNop(), []string{"http://localhost:3000"})
	return r, cookieConfig, jwtService
}

func TestAuthHandlerRejectsMalformedCredentials(t *testing.T) {
	r, _, _ := setupAuthRouter(t)

	for _, path := range []string{"/api/auth/login", "/api/auth/register"} {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"email":"not-an-email"}`))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		r.ServeHTTP(response, req)

		assert.Equal(t, http.StatusBadRequest, response.Code, path)
	}
}

func TestAuthHandlerValidateToken(t *testing.T) {
	r, _, jwtService := setupAuthRouter(t)
	token, err := jwtService.GenerateToken("user-123")
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", bytes.NewBufferString(`{"token":"`+token+`"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)

	assert.Equal(t, http.StatusOK, response.Code)
	var body models.TokenValidationResponse
	assert.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.True(t, body.Valid)
	assert.Equal(t, "user-123", body.UserID)
}

func TestAuthHandlerValidateTokenRequiresToken(t *testing.T) {
	r, _, _ := setupAuthRouter(t)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil))

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"token is required"}`, response.Body.String())
}

func TestAuthHandlerRefreshTokenFromAuthorizationHeader(t *testing.T) {
	r, cookieConfig, jwtService := setupAuthRouter(t)
	token, err := jwtService.GenerateToken("user-123")
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Set-Cookie"), cookieConfig.Name+"=")
	assert.JSONEq(t, `{"status":"success"}`, response.Body.String())
}

func TestAuthHandlerRefreshTokenRejectsMissingOrInvalidToken(t *testing.T) {
	r, _, _ := setupAuthRouter(t)

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil),
		httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewBufferString(`{"token":"invalid"}`)),
	} {
		if request.Body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	}
}

func TestAuthHandlerMeUsesCookieBeforeOtherTokenSources(t *testing.T) {
	r, cookieConfig, jwtService := setupAuthRouter(t)
	cookieToken, err := jwtService.GenerateToken("cookie-user")
	assert.NoError(t, err)
	headerToken, err := jwtService.GenerateToken("header-user")
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: cookieConfig.Name, Value: cookieToken})
	req.Header.Set("Authorization", "Bearer "+headerToken)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"success","user_id":"cookie-user"}`, response.Body.String())
}

func TestAuthHandlerMeRejectsMissingOrInvalidToken(t *testing.T) {
	r, _, _ := setupAuthRouter(t)

	for _, testCase := range []struct {
		name    string
		request *http.Request
	}{
		{name: "missing token", request: httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)},
		{name: "invalid token", request: func() *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
			req.Header.Set("Authorization", "Bearer invalid")
			return req
		}()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, testCase.request)
			assert.Equal(t, http.StatusUnauthorized, response.Code)
		})
	}
}

func TestAuthHandlerDeleteAccountRejectsMissingOrInvalidToken(t *testing.T) {
	r, _, _ := setupAuthRouter(t)

	tests := []struct {
		name    string
		request *http.Request
	}{
		{name: "missing token", request: httptest.NewRequest(http.MethodDelete, "/api/auth/account", nil)},
		{name: "invalid token", request: func() *http.Request {
			request := httptest.NewRequest(http.MethodDelete, "/api/auth/account", nil)
			request.Header.Set("Authorization", "Bearer invalid")
			return request
		}()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, test.request)
			assert.Equal(t, http.StatusUnauthorized, response.Code)
		})
	}
}

func TestAuthHandlerLogoutClearsCookie(t *testing.T) {
	r, cookieConfig, _ := setupAuthRouter(t)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Set-Cookie"), cookieConfig.Name+"=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax")
	assert.JSONEq(t, `{"status":"success","message":"logged out"}`, response.Body.String())
}

func TestAuthHandlerMeReturnsUserFromAuthorizationHeader(t *testing.T) {
	r, _, jwtService := setupAuthRouter(t)
	token, err := jwtService.GenerateToken("header-user")
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"success","user_id":"header-user"}`, response.Body.String())
}

func TestAuthHandlerRefreshTokenUsesCookieWhenNoAuthorizationHeader(t *testing.T) {
	r, cookieConfig, jwtService := setupAuthRouter(t)
	token, err := jwtService.GenerateToken("cookie-user")
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: cookieConfig.Name, Value: token})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Set-Cookie"), cookieConfig.Name+"=")
	assert.JSONEq(t, `{"status":"success"}`, response.Body.String())
}
