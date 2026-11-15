package config

import (
	"os"
	"strings"
)

// Config holds all configuration for the auth service
type Config struct {
	Port                  string
	MongoURI              string
	MongoDB               string
	JWTSecret             string
	KafkaBrokers          string
	KafkaClientID         string
	KafkaTopicUserCreated string
	KafkaTopicUserUpdated string
	KafkaTopicUserDeleted string
	CookieSecure          bool
	CookieMaxAge          int
	CookieSameSite        string
	CookieDomain          string
	SMTPHost              string
	SMTPPort              string
	SMTPUsername          string
	SMTPPassword          string
	EmailFrom             string
	FrontendURL           string
	AllowedOrigins        []string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		Port:                  getEnv("PORT", "8081"),
		MongoURI:              getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:               getEnv("MONGO_DB", "auth_db"),
		JWTSecret:             getEnv("JWT_SECRET", "your-secret-key"),
		KafkaBrokers:          getEnv("KAFKA_BROKERS", ""),
		KafkaClientID:         getEnv("KAFKA_CLIENT_ID", "auth-service"),
		KafkaTopicUserCreated: getEnv("KAFKA_TOPIC_USER_CREATED", "user.created.v1"),
		KafkaTopicUserUpdated: getEnv("KAFKA_TOPIC_USER_UPDATED", "user.updated.v1"),
		KafkaTopicUserDeleted: getEnv("KAFKA_TOPIC_USER_DELETED", "user.deleted.v1"),
		CookieSecure:          parseBoolEnv(os.Getenv("COOKIE_SECURE"), false),
		CookieMaxAge:          parseIntEnv(os.Getenv("COOKIE_MAX_AGE"), 24*60*60),
		CookieSameSite:        getEnv("COOKIE_SAME_SITE", "Lax"),
		CookieDomain:          getEnv("COOKIE_DOMAIN", ""),
		SMTPHost:              getEnv("SMTP_HOST", ""),
		SMTPPort:              getEnv("SMTP_PORT", "587"),
		SMTPUsername:          getEnv("SMTP_USERNAME", ""),
		SMTPPassword:          getEnv("SMTP_PASSWORD", ""),
		EmailFrom:             getEnv("EMAIL_FROM", ""),
		FrontendURL:           strings.TrimRight(getEnv("FRONTEND_URL", "http://localhost:4200"), "/"),
		AllowedOrigins:        splitCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:4200,http://localhost:8085")),
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
