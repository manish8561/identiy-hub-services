package config

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCookieConfig(t *testing.T) {
	tests := []struct {
		name       string
		sameSite   string
		expected   http.SameSite
		wantSecure bool
	}{
		{name: "strict", sameSite: "strict", expected: http.SameSiteStrictMode, wantSecure: true},
		{name: "none", sameSite: "NONE", expected: http.SameSiteNoneMode, wantSecure: true},
		{name: "default", sameSite: "unknown", expected: http.SameSiteLaxMode, wantSecure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cookie := NewCookieConfig(true, 0, tt.sameSite, "example.test")
			assert.Equal(t, AccessTokenCookieName, cookie.Name)
			assert.Equal(t, 24*60*60, cookie.MaxAge)
			assert.Equal(t, tt.wantSecure, cookie.Secure)
			assert.True(t, cookie.HTTPOnly)
			assert.Equal(t, "/", cookie.Path)
			assert.Equal(t, "example.test", cookie.Domain)
			assert.Equal(t, tt.expected, cookie.SameSite)
		})
	}
}

// TestNewCookieConfigNoneForceSecure verifies that SameSite=None always
// promotes Secure to true regardless of the caller's setting, as required
// by RFC 6265bis / browser spec.
func TestNewCookieConfigNoneForceSecure(t *testing.T) {
	cookie := NewCookieConfig(false, 3600, "None", "")
	assert.Equal(t, http.SameSiteNoneMode, cookie.SameSite)
	assert.True(t, cookie.Secure, "SameSite=None must force Secure=true")
}

func TestConfigHelpers(t *testing.T) {
	assert.True(t, parseBoolEnv("true", false))
	assert.False(t, parseBoolEnv("invalid", false))
	assert.Equal(t, 42, parseIntEnv("42", 1))
	assert.Equal(t, 1, parseIntEnv("invalid", 1))
	assert.Equal(t, []string{"https://one.test", "https://two.test"}, splitCSV(" https://one.test, ,https://two.test "))
}

func TestLoadConfigUsesDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("MONGO_URI", "")
	t.Setenv("MONGO_DB", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("KAFKA_CLIENT_ID", "")
	t.Setenv("KAFKA_TOPIC_USER_CREATED", "")
	t.Setenv("KAFKA_TOPIC_USER_UPDATED", "")
	t.Setenv("KAFKA_TOPIC_USER_DELETED", "")
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("COOKIE_MAX_AGE", "")
	t.Setenv("COOKIE_SAME_SITE", "")
	t.Setenv("COOKIE_DOMAIN", "")
	t.Setenv("ALLOWED_ORIGINS", "")

	cfg := LoadConfig()

	assert.Equal(t, "8081", cfg.Port)
	assert.Equal(t, "mongodb://localhost:27017", cfg.MongoURI)
	assert.Equal(t, "auth_db", cfg.MongoDB)
	assert.Equal(t, "your-secret-key", cfg.JWTSecret)
	assert.Equal(t, "", cfg.KafkaBrokers)
	assert.Equal(t, "auth-service", cfg.KafkaClientID)
	assert.Equal(t, "user.created.v1", cfg.KafkaTopicUserCreated)
	assert.Equal(t, "user.updated.v1", cfg.KafkaTopicUserUpdated)
	assert.Equal(t, "user.deleted.v1", cfg.KafkaTopicUserDeleted)
	assert.False(t, cfg.CookieSecure)
	assert.Equal(t, 24*60*60, cfg.CookieMaxAge)
	assert.Equal(t, "Lax", cfg.CookieSameSite)
	assert.Empty(t, cfg.CookieDomain)
	assert.Equal(t, []string{"http://localhost:4200", "http://localhost:8085"}, cfg.AllowedOrigins)
}

func TestLoadConfigReadsEnvironmentOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("MONGO_URI", "mongodb://mongo.example:27017")
	t.Setenv("MONGO_DB", "auth_prod")
	t.Setenv("JWT_SECRET", "super-secret")
	t.Setenv("KAFKA_BROKERS", "broker-1:9092,broker-2:9092")
	t.Setenv("KAFKA_CLIENT_ID", "auth-worker")
	t.Setenv("KAFKA_TOPIC_USER_CREATED", "custom.created")
	t.Setenv("KAFKA_TOPIC_USER_UPDATED", "custom.updated")
	t.Setenv("KAFKA_TOPIC_USER_DELETED", "custom.deleted")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("COOKIE_MAX_AGE", "3600")
	t.Setenv("COOKIE_SAME_SITE", "strict")
	t.Setenv("COOKIE_DOMAIN", "example.com")
	t.Setenv("ALLOWED_ORIGINS", " https://app.example.com, , https://admin.example.com ")

	cfg := LoadConfig()

	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "mongodb://mongo.example:27017", cfg.MongoURI)
	assert.Equal(t, "auth_prod", cfg.MongoDB)
	assert.Equal(t, "super-secret", cfg.JWTSecret)
	assert.Equal(t, "broker-1:9092,broker-2:9092", cfg.KafkaBrokers)
	assert.Equal(t, "auth-worker", cfg.KafkaClientID)
	assert.Equal(t, "custom.created", cfg.KafkaTopicUserCreated)
	assert.Equal(t, "custom.updated", cfg.KafkaTopicUserUpdated)
	assert.Equal(t, "custom.deleted", cfg.KafkaTopicUserDeleted)
	assert.True(t, cfg.CookieSecure)
	assert.Equal(t, 3600, cfg.CookieMaxAge)
	assert.Equal(t, "strict", cfg.CookieSameSite)
	assert.Equal(t, "example.com", cfg.CookieDomain)
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, cfg.AllowedOrigins)
}

func TestMongoDBConfigCloseHandlesNilAndClosedClient(t *testing.T) {
	var cfg *MongoDBConfig
	assert.NoError(t, cfg.Close())

	cfg = &MongoDBConfig{}
	assert.NoError(t, cfg.Close())
}
