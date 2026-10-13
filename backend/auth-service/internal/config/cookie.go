package config

import (
	"net/http"
	"strconv"
	"strings"
)

const AccessTokenCookieName = "access_token"

// CookieConfig holds HTTP cookie settings for auth tokens.
type CookieConfig struct {
	Name     string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	Path     string
	Domain   string
	SameSite http.SameSite
}

// NewCookieConfig builds cookie settings from environment-backed values.
//
// Important: SameSite=None requires the Secure flag to be set; browsers will
// reject the cookie otherwise (RFC 6265bis). If sameSite resolves to None and
// secure is false, secure is promoted to true automatically.
func NewCookieConfig(secure bool, maxAge int, sameSite, domain string) *CookieConfig {
	ss := http.SameSiteLaxMode
	switch strings.ToLower(strings.TrimSpace(sameSite)) {
	case "strict":
		ss = http.SameSiteStrictMode
	case "none":
		ss = http.SameSiteNoneMode
		// SameSite=None is only valid over HTTPS; force Secure regardless of config.
		secure = true
	case "lax", "":
		ss = http.SameSiteLaxMode
	}

	if maxAge <= 0 {
		maxAge = 24 * 60 * 60
	}

	return &CookieConfig{
		Name:     AccessTokenCookieName,
		MaxAge:   maxAge,
		Secure:   secure,
		HTTPOnly: true,
		Path:     "/",
		Domain:   domain,
		SameSite: ss,
	}
}

func parseBoolEnv(value string, defaultValue bool) bool {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseIntEnv(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
