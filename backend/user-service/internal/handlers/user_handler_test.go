package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"user-service/internal/handlers"
	"user-service/internal/middleware"
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

func setupTestRouter(mockUserService *MockUserService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Create a mock logger
	logger, _ := zap.NewDevelopment()
	userHandler := handlers.NewUserHandler(mockUserService, logger)

	// Add middleware to set user ID context for testing (bypass auth)
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextUserIDKey, "123")
		c.Next()
	})

	api := r.Group("/api/users")
	{
		api.GET("/me", userHandler.GetCurrentUser)
		api.GET("/profile/:id", userHandler.GetUserByID)
		api.GET("/list", userHandler.ListUsers)
		api.PUT("/profile/:id", userHandler.UpdateUser)
		api.DELETE("/profile/:id", userHandler.DeleteUser)
	}

	return r
}

func TestGetCurrentUser_Success(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	expectedUser := &models.User{
		ID:    "123",
		Name:  "testuser",
		Email: "test@example.com",
	}

	mockUserService.On("GetUserByID", "123").Return(expectedUser, nil)

	req, _ := http.NewRequest("GET", "/api/users/me", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockUserService.AssertExpectations(t)
}

func TestGetUserByID_Success(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	expectedUser := &models.User{
		ID:    "123",
		Name:  "testuser",
		Email: "test@example.com",
	}

	mockUserService.On("GetUserByID", "123").Return(expectedUser, nil)

	req, _ := http.NewRequest("GET", "/api/users/profile/123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.User
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, expectedUser.ID, response.ID)

	mockUserService.AssertExpectations(t)
}

func TestUpdateUser_Success(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	updateReq := models.UpdateUserRequest{
		Name:  "updateduser",
		Email: "updated@example.com",
		Role:  "customer",
	}

	expectedUser := &models.User{
		ID:    "123",
		Name:  "updateduser",
		Email: "updated@example.com",
	}

	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "customer"}, nil)
	mockUserService.On("UpdateUser", "123", updateReq).Return(expectedUser, nil)

	reqBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", "/api/users/profile/123", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.User
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, expectedUser.Name, response.Name)

	mockUserService.AssertExpectations(t)
}

func TestDeleteUser_RejectsDeletingAdminSelf(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "admin"}, nil)

	req, _ := http.NewRequest("DELETE", "/api/users/profile/123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"admin users cannot be deleted"}`, w.Body.String())
	mockUserService.AssertExpectations(t)
	mockUserService.AssertNotCalled(t, "DeleteUser", mock.Anything)
}

func TestListUsers_Success(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	expectedResponse := &models.UserListResponse{
		Users: []models.User{
			{ID: "1", Name: "user1", Email: "user1@example.com"},
			{ID: "2", Name: "user2", Email: "user2@example.com"},
		},
		Page:       1,
		Size:       10,
		TotalCount: 2,
		Total:      2,
	}

	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "admin"}, nil)
	mockUserService.On("ListUsers", 1, 10).Return(expectedResponse, nil)

	req, _ := http.NewRequest("GET", "/api/users/list?page=1&size=10", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.UserListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(response.Users))

	mockUserService.AssertExpectations(t)
}

func TestGetUserByID_RejectsMismatchedUserID(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	req, _ := http.NewRequest("GET", "/api/users/profile/other-user", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"forbidden"}`, w.Body.String())
}

func TestUpdateUser_RejectsInvalidJSON(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	req, _ := http.NewRequest("PUT", "/api/users/profile/123", bytes.NewBufferString(`{"name":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestListUsers_DefaultsInvalidPaginationValues(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)

	expectedResponse := &models.UserListResponse{Users: []models.User{{ID: "123", Name: "testuser", Email: "test@example.com"}}, Page: 1, Size: 10, TotalCount: 1, Total: 1}
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "admin"}, nil)
	mockUserService.On("ListUsers", 1, 10).Return(expectedResponse, nil)

	req, _ := http.NewRequest("GET", "/api/users/list?page=0&size=9999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockUserService.AssertExpectations(t)
}

func TestDeleteUser_RejectsMissingUserIDParam(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := handlers.NewUserHandler(new(MockUserService), logger)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/users/profile/", nil)
	c.Set(middleware.ContextUserIDKey, "123")
	c.Params = gin.Params{{Key: "id", Value: ""}}

	handler.DeleteUser(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"user ID is required"}`, w.Body.String())
}

func TestListUsers_RejectsNonAdmin(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "customer"}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users/list", nil))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"forbidden"}`, w.Body.String())
	mockUserService.AssertExpectations(t)
	mockUserService.AssertNotCalled(t, "ListUsers", mock.Anything, mock.Anything)
}

func TestUpdateUser_RejectsNonAdminUpdatingAnotherUser(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "customer"}, nil)

	body, _ := json.Marshal(models.UpdateUserRequest{
		Name:  "another user",
		Email: "another@example.com",
		Role:  "customer",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/users/profile/other-user", bytes.NewReader(body)))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"forbidden"}`, w.Body.String())
	mockUserService.AssertExpectations(t)
	mockUserService.AssertNotCalled(t, "UpdateUser", mock.Anything, mock.Anything)
}

func TestDeleteUser_RejectsNonAdminDeletingAnotherUser(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "customer"}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/users/profile/other-user", nil))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"forbidden"}`, w.Body.String())
	mockUserService.AssertExpectations(t)
	mockUserService.AssertNotCalled(t, "DeleteUser", mock.Anything)
}

func TestDeleteUser_RejectsNonAdminDeletingOwnAccount(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "customer"}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/users/profile/123", nil))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"forbidden"}`, w.Body.String())
	mockUserService.AssertExpectations(t)
	mockUserService.AssertNotCalled(t, "DeleteUser", mock.Anything)
}

func TestUpdateUser_RejectsSelfPromotion(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "customer"}, nil)

	body, _ := json.Marshal(models.UpdateUserRequest{
		Name:  "testuser",
		Email: "test@example.com",
		Role:  "admin",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/users/profile/123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"forbidden"}`, w.Body.String())
	mockUserService.AssertExpectations(t)
	mockUserService.AssertNotCalled(t, "UpdateUser", mock.Anything, mock.Anything)
}

func TestUserResponseOmitsPassword(t *testing.T) {
	user := models.User{ID: "123", Password: "hashed-password"}
	body, err := json.Marshal(user)

	assert.NoError(t, err)
	assert.NotContains(t, string(body), "password")
	assert.NotContains(t, string(body), "hashed-password")
}

func TestUpdateUser_AdminCanUpdateAnotherUser(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	request := models.UpdateUserRequest{
		Name:  "Updated customer",
		Email: "updated@example.com",
		Role:  "customer",
	}
	updatedUser := &models.User{
		ID:    "other-user",
		Name:  request.Name,
		Email: request.Email,
		Role:  request.Role,
	}
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "admin"}, nil)
	mockUserService.On("UpdateUser", "other-user", request).Return(updatedUser, nil)

	body, _ := json.Marshal(request)
	req := httptest.NewRequest(http.MethodPut, "/api/users/profile/other-user", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockUserService.AssertExpectations(t)
}

func TestDeleteUser_AdminCanDeleteAnotherUser(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "admin"}, nil)
	mockUserService.On("GetUserByID", "other-user").Return(&models.User{ID: "other-user", Role: "customer"}, nil)
	mockUserService.On("DeleteUser", "other-user").Return(nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/users/profile/other-user", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	mockUserService.AssertExpectations(t)
}

func TestDeleteUser_RejectsDeletingAnotherAdmin(t *testing.T) {
	mockUserService := new(MockUserService)
	r := setupTestRouter(mockUserService)
	mockUserService.On("GetUserByID", "123").Return(&models.User{ID: "123", Role: "admin"}, nil)
	mockUserService.On("GetUserByID", "other-admin").Return(&models.User{ID: "other-admin", Role: "admin"}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/users/profile/other-admin", nil))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"admin users cannot be deleted"}`, w.Body.String())
	mockUserService.AssertExpectations(t)
	mockUserService.AssertNotCalled(t, "DeleteUser", mock.Anything)
}
