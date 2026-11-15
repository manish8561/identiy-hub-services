package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// AccessTokenCookieName is the HttpOnly auth cookie name.
	AccessTokenCookieName = "access_token"
	// ContextUserIDKey stores the authenticated user id on the request context.
	ContextUserIDKey = "user_id"
)

// CookieAuth validates the access token cookie (with Bearer fallback) and sets user_id.
func CookieAuth(jwtSecret string) gin.HandlerFunc {
	return CookieAuthWithAccountStatus(jwtSecret, nil)
}

type AccountStatusChecker func(userID, sessionID string) (bool, error)

func CookieAuthWithAccountStatus(jwtSecret string, accountActive AccountStatusChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization token is required"})
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}

		userID, _ := claims["user_id"].(string)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID in token"})
			return
		}
		if accountActive != nil {
			sessionID, _ := claims["jti"].(string)
			if sessionID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid session ID in token"})
				return
			}
			active, err := accountActive(userID, sessionID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to validate account status"})
				return
			}
			if !active {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "account is inactive or email is unverified"})
				return
			}
		}

		c.Set(ContextUserIDKey, userID)
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	if cookie, err := c.Cookie(AccessTokenCookieName); err == nil && cookie != "" {
		return cookie
	}

	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}
