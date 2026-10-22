package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"user-service/internal/handlers"
	"user-service/internal/models"
)

// MockUserService is a mock implementation of UserService for testing
type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetUserByID(id string) (*models.User, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserService) ListUsers(page, size int) (*models.UserListResponse, error) {
	args := m.Called(page, size)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserListResponse), args.Error(1)
}

func (m *MockUserService) UpdateUser(id string, req models.UpdateUserRequest) (*models.User, error) {
	args := m.Called(id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserService) DeleteUser(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockUserService) CreateUser(req models.CreateUserRequest) (*models.User, error) {
	args := m.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserService) UpsertUserProfileFromEvent(event models.UserEvent) error {
	args := m.Called(event)
	return args.Error(0)
}

func (m *MockUserService) DeleteUserProfileFromEvent(event models.UserEvent) error {
	args := m.Called(event)
	return args.Error(0)
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := zap.NewNop()
	mockUserService := new(MockUserService)
	userHandler := handlers.NewUserHandler(mockUserService, log)

	return NewRouter(userHandler, log, "test-secret", []string{"http://localhost:3000"})
}

func TestNewRouter_RegistersHealthEndpoint(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")
	assert.Contains(t, w.Body.String(), "user-service")
}

func TestNewRouter_RegistersUserRoutes(t *testing.T) {
	router := setupTestRouter()

	testCases := []struct {
		method       string
		path         string
		expectedCode int
	}{
		{http.MethodGet, "/api/users/me", http.StatusUnauthorized},             // Will fail due to missing auth
		{http.MethodGet, "/api/users/profile/123", http.StatusUnauthorized},    // Will fail due to missing auth
		{http.MethodGet, "/api/users/list", http.StatusUnauthorized},           // Will fail due to missing auth
		{http.MethodPut, "/api/users/profile/123", http.StatusUnauthorized},    // Will fail due to missing auth
		{http.MethodDelete, "/api/users/profile/123", http.StatusUnauthorized}, // Will fail due to missing auth
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

func TestNewRouter_AppliesAuthMiddleware(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 401 because no auth token is provided
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNewRouter_AppliesLoggingMiddleware(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// If logging middleware is applied, the request should be processed
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestNewRouter_RateLimitsUserAPIByClientIP(t *testing.T) {
	router := setupTestRouter()

	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
