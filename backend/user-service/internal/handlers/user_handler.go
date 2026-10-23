package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"user-service/internal/logger"
	"user-service/internal/middleware"
	"user-service/internal/models"
	"user-service/internal/services"
)

// UserHandler handles HTTP requests for user operations
type UserHandler struct {
	userService services.UserServiceInterface
	logger      logger.Logger
}

// NewUserHandler creates a new UserHandler with the provided user service and logger
func NewUserHandler(userService services.UserServiceInterface, logger logger.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      logger,
	}
}

// GetCurrentUser handles requests for the authenticated user's profile.
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	userID, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, _ := userID.(string)
	h.logger.Info("Getting current user profile",
		zap.String("user_id", id),
		zap.String("client_ip", c.ClientIP()),
	)

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		h.logger.Warn("User not found",
			zap.String("user_id", id),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// GetUserByID handles requests to get a user profile by ID (must match the authenticated user).
func (h *UserHandler) GetUserByID(c *gin.Context) {
	authUserID, _ := c.Get(middleware.ContextUserIDKey)
	authID, _ := authUserID.(string)
	id := c.Param("id")

	if id == "" || id == "me" {
		id = authID
	}

	if id != authID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	h.logger.Info("Getting user by ID",
		zap.String("user_id", id),
		zap.String("client_ip", c.ClientIP()),
	)

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		h.logger.Warn("User not found",
			zap.String("user_id", id),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("User retrieved successfully",
		zap.String("user_id", id),
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, user)
}

// ListUsers handles requests to list users with pagination
func (h *UserHandler) ListUsers(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	sizeStr := c.DefaultQuery("size", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil || size < 1 || size > 100 {
		size = 10
	}

	h.logger.Info("Listing users",
		zap.Int("page", page),
		zap.Int("size", size),
		zap.String("client_ip", c.ClientIP()),
	)

	response, err := h.userService.ListUsers(page, size)
	if err != nil {
		h.logger.Error("Failed to list users",
			zap.Error(err),
			zap.Int("page", page),
			zap.Int("size", size),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("Users listed successfully",
		zap.Int("page", page),
		zap.Int("size", size),
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, response)
}

// UpdateUser handles requests to update a user
func (h *UserHandler) UpdateUser(c *gin.Context) {
	authUserID, _ := c.Get(middleware.ContextUserIDKey)
	authID, _ := authUserID.(string)
	id := c.Param("id")
	if id == "" {
		h.logger.Error("Missing user ID in update request",
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "user ID is required"})
		return
	}

	if id != authID {
		if !h.requireAdmin(c) {
			return
		}
	}

	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("Failed to bind update user request",
			zap.Error(err),
			zap.String("user_id", id),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role != "customer" && req.Role != "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be customer or admin"})
		return
	}

	if id == authID {
		user, err := h.userService.GetUserByID(authID)
		if err != nil || user == nil {
			h.logger.Error("Failed to verify user role",
				zap.String("user_id", authID),
				zap.Error(err),
				zap.String("client_ip", c.ClientIP()),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify user permissions"})
			return
		}
		if req.Role != user.Role && user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
	}

	h.logger.Info("Updating user",
		zap.String("user_id", id),
		zap.String("client_ip", c.ClientIP()),
	)

	user, err := h.userService.UpdateUser(id, req)
	if err != nil {
		h.logger.Warn("Failed to update user",
			zap.String("user_id", id),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("User updated successfully",
		zap.String("user_id", id),
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, user)
}

// DeleteUser handles requests to delete a user
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		h.logger.Error("Missing user ID in delete request",
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "user ID is required"})
		return
	}

	if !h.requireAdmin(c) {
		return
	}
	targetUser, err := h.userService.GetUserByID(id)
	if err != nil || targetUser == nil {
		h.logger.Warn("Failed to verify delete target",
			zap.String("user_id", id),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if targetUser.Role == "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin users cannot be deleted"})
		return
	}

	h.logger.Info("Deleting user",
		zap.String("user_id", id),
		zap.String("client_ip", c.ClientIP()),
	)

	err = h.userService.DeleteUser(id)
	if err != nil {
		h.logger.Warn("Failed to delete user",
			zap.String("user_id", id),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("User deleted successfully",
		zap.String("user_id", id),
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

func (h *UserHandler) requireAdmin(c *gin.Context) bool {
	authUserID, ok := c.Get(middleware.ContextUserIDKey)
	authID, validID := authUserID.(string)
	if !ok || !validID || authID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}

	user, err := h.userService.GetUserByID(authID)
	if err != nil || user == nil {
		h.logger.Error("Failed to verify user role",
			zap.String("user_id", authID),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to verify user permissions"})
		return false
	}
	if user.Role != "admin" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return false
	}
	return true
}
