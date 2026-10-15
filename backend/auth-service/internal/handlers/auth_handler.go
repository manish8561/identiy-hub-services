package handlers

import (
	"auth-service/internal/config"
	"auth-service/internal/logger"
	"auth-service/internal/models"
	"auth-service/internal/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthHandler handles HTTP requests for authentication operations
type AuthHandler struct {
	authService  *services.AuthService
	logger       logger.Logger
	cookieConfig *config.CookieConfig
}

// NewAuthHandler creates a new AuthHandler with the provided auth service
func NewAuthHandler(authService *services.AuthService, logger logger.Logger, cookieConfig *config.CookieConfig) *AuthHandler {
	return &AuthHandler{
		authService:  authService,
		logger:       logger,
		cookieConfig: cookieConfig,
	}
}

// Login handles user login requests
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("Failed to bind login request",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("Login attempt",
		zap.String("email", req.Email),
		zap.String("client_ip", c.ClientIP()),
	)

	response, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		h.logger.Warn("Login failed",
			zap.String("email", req.Email),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	h.setAccessTokenCookie(c, response.Token)
	h.logger.Info("Login successful",
		zap.String("email", req.Email),
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, models.LoginResponse{
		Status: "success",
	})
}

// Register handles user registration requests
func (h *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("Failed to bind register request",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("Registration attempt",
		zap.String("email", req.Email),
		zap.String("client_ip", c.ClientIP()),
	)

	response, err := h.authService.Register(req)
	if err != nil {
		h.logger.Warn("Registration failed",
			zap.String("email", req.Email),
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.setAccessTokenCookie(c, response.Token)
	h.logger.Info("Registration successful",
		zap.String("email", req.Email),
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusCreated, models.RegisterResponse{
		Status:  "success",
		Message: response.Message,
	})
}

// ValidateToken handles token validation requests
func (h *AuthHandler) ValidateToken(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	h.logger.Debug("Token validation attempt",
		zap.String("client_ip", c.ClientIP()),
	)

	response, err := h.authService.ValidateToken(token)
	if err != nil {
		h.logger.Warn("Token validation failed",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Debug("Token validation successful",
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, response)
}

// RefreshToken handles token refresh requests
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	h.logger.Debug("Token refresh attempt",
		zap.String("client_ip", c.ClientIP()),
	)

	response, err := h.authService.RefreshToken(token)
	if err != nil {
		h.logger.Warn("Token refresh failed",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	h.setAccessTokenCookie(c, response.Token)
	h.logger.Info("Token refresh successful",
		zap.String("client_ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, models.RefreshTokenResponse{
		Status: "success",
	})
}

// Me returns the current session user from the access token cookie.
func (h *AuthHandler) Me(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	response, err := h.authService.ValidateToken(token)
	if err != nil || !response.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"user_id": response.UserID,
	})
}

// DeleteAccount removes the account associated with the current session.
func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	response, err := h.authService.ValidateToken(token)
	if err != nil || !response.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.authService.DeleteAccount(response.UserID); err != nil {
		h.logger.Error("Failed to delete account",
			zap.String("user_id", response.UserID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.clearAccessTokenCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "account deleted"})
}

// Logout clears the access token cookie.
func (h *AuthHandler) Logout(c *gin.Context) {
	h.clearAccessTokenCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "logged out"})
}

func (h *AuthHandler) extractToken(c *gin.Context) string {
	if cookie, err := c.Cookie(h.cookieConfig.Name); err == nil && cookie != "" {
		return cookie
	}

	var bodyToken struct {
		Token string `json:"token"`
	}
	_ = c.ShouldBindJSON(&bodyToken)
	if bodyToken.Token != "" {
		return bodyToken.Token
	}

	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}

func (h *AuthHandler) setAccessTokenCookie(c *gin.Context, token string) {
	c.SetSameSite(h.cookieConfig.SameSite)
	c.SetCookie(
		h.cookieConfig.Name,
		token,
		h.cookieConfig.MaxAge,
		h.cookieConfig.Path,
		h.cookieConfig.Domain,
		h.cookieConfig.Secure,
		h.cookieConfig.HTTPOnly,
	)
}

func (h *AuthHandler) clearAccessTokenCookie(c *gin.Context) {
	c.SetSameSite(h.cookieConfig.SameSite)
	c.SetCookie(
		h.cookieConfig.Name,
		"",
		-1,
		h.cookieConfig.Path,
		h.cookieConfig.Domain,
		h.cookieConfig.Secure,
		h.cookieConfig.HTTPOnly,
	)
}
