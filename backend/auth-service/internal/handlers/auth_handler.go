package handlers

import (
	"auth-service/internal/config"
	"auth-service/internal/logger"
	"auth-service/internal/models"
	"auth-service/internal/services"
	"errors"
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
		status := http.StatusUnauthorized
		if isSessionStoreError(err) {
			status = http.StatusInternalServerError
		} else if errors.Is(err, services.ErrEmailNotVerified) || errors.Is(err, services.ErrAccountSuspended) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
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
		status := http.StatusBadRequest
		var deliveryErr *services.EmailDeliveryError
		if isSessionStoreError(err) || errors.As(err, &deliveryErr) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": err.Error()})
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
		status := http.StatusUnauthorized
		if isSessionStoreError(err) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": err.Error()})
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
	if err != nil {
		h.logger.Error("Failed to validate current session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate session"})
		return
	}
	if !response.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"user_id": response.UserID,
	})
}

// ChangePassword updates the authenticated user's password.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req models.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.authService.ChangePassword(token, req.CurrentPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidCurrentPassword):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrInvalidSession):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		case errors.Is(err, services.ErrPasswordUnchanged), errors.Is(err, services.ErrPasswordTooLong):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case isSessionStoreError(err):
			h.logger.Error("Failed to revoke other sessions after password change", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "password changed but failed to revoke other sessions"})
		default:
			h.logger.Error("Failed to change password", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change password"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "password changed"})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req models.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.VerifyEmail(req.Token); err != nil {
		h.writeActionError(c, "verify email", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "email verified"})
}

func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req models.EmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.ResendVerification(req.Email); err != nil {
		var deliveryErr *services.EmailDeliveryError
		if errors.As(err, &deliveryErr) {
			h.logger.Error("Verification email delivery failed", zap.Error(deliveryErr))
			c.JSON(http.StatusOK, gin.H{"status": "success", "message": "if the account needs verification, an email has been sent"})
			return
		}
		h.logger.Error("Failed to send verification email", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not send verification email"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "if the account needs verification, an email has been sent"})
}

func (h *AuthHandler) RequestPasswordReset(c *gin.Context) {
	var req models.EmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.RequestPasswordReset(req.Email); err != nil {
		var deliveryErr *services.EmailDeliveryError
		if errors.As(err, &deliveryErr) {
			h.logger.Error("Password reset email delivery failed", zap.Error(deliveryErr))
			c.JSON(http.StatusOK, gin.H{"status": "success", "message": "if the account exists, password reset instructions have been sent"})
			return
		}
		h.logger.Error("Failed to request password reset", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not process password reset request"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "if the account exists, password reset instructions have been sent"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req models.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.ResetPassword(req.Token, req.NewPassword); err != nil {
		h.writeActionError(c, "reset password", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "password reset"})
}

func (h *AuthHandler) SetAccountStatus(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	validation, err := h.authService.ValidateToken(token)
	if err != nil || !validation.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req models.AccountStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.SetAccountStatus(validation.UserID, c.Param("id"), req.Status); err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case isSessionStoreError(err):
			h.logger.Error("Failed to revoke sessions for account status change", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update account status"})
		default:
			h.logger.Error("Failed to update account status", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "account status updated"})
}

func (h *AuthHandler) UpdateUserByAdmin(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	validation, err := h.authService.ValidateToken(token)
	if err != nil || !validation.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req models.AdminUpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.UpdateUserByAdmin(validation.UserID, c.Param("id"), req); err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			h.logger.Error("Failed to update user account", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "user updated"})
}

func (h *AuthHandler) DeleteUserByAdmin(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	validation, err := h.authService.ValidateToken(token)
	if err != nil || !validation.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.authService.DeleteUserByAdmin(validation.UserID, c.Param("id")); err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrInvalidActionToken):
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		case isSessionStoreError(err):
			h.logger.Error("Failed to revoke sessions before deleting user", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		default:
			h.logger.Error("Failed to delete user account", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "user deleted"})
}

func (h *AuthHandler) UpdateOwnProfile(c *gin.Context) {
	var req models.UpdateOwnProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	emailChanged, err := h.authService.UpdateOwnProfile(token, req.Name, req.Email, req.CurrentPassword)
	if err != nil {
		if emailChanged {
			h.clearAccessTokenCookie(c)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":                "email changed but verification could not be completed; use the login page to resend verification",
				"verificationRequired": true,
			})
			return
		}
		switch {
		case errors.Is(err, services.ErrInvalidCurrentPassword):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrInvalidSession):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		case isSessionStoreError(err):
			h.logger.Error("Failed to revoke sessions after profile update", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		default:
			h.logger.Error("Failed to update current profile", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	if emailChanged {
		h.clearAccessTokenCookie(c)
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success", "message": "profile updated", "verificationRequired": emailChanged,
	})
}

// DeleteAccount removes the account associated with the current session.
func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	token := h.extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	validation, err := h.authService.ValidateToken(token)
	if err != nil || !validation.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req models.DeleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.authService.DeleteAccount(token, req.CurrentPassword); err != nil {
		h.logger.Error("Failed to delete account",
			zap.String("user_id", validation.UserID),
			zap.Error(err),
		)
		switch {
		case errors.Is(err, services.ErrInvalidCurrentPassword):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrInvalidSession):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	h.clearAccessTokenCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "account deleted"})
}

func (h *AuthHandler) writeActionError(c *gin.Context, action string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidActionToken):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case isSessionStoreError(err):
		h.logger.Error("Failed to complete "+action, zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to " + action})
	default:
		h.logger.Error("Failed to complete "+action, zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to " + action})
	}
}

// Logout revokes the current server-side session and clears the access token cookie.
func (h *AuthHandler) Logout(c *gin.Context) {
	if err := h.authService.Logout(h.extractToken(c)); err != nil {
		h.logger.Error("Failed to revoke logout session",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		h.clearAccessTokenCookie(c)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to log out"})
		return
	}
	h.clearAccessTokenCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "logged out"})
}

func isSessionStoreError(err error) bool {
	var sessionErr *services.SessionStoreError
	return errors.As(err, &sessionErr)
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
