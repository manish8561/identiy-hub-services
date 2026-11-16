package config

import (
	"os"
	"strings"
)

type Config struct {
	Port             string
	MongoURI         string
	MongoDB          string
	JWTSecret        string
	KafkaBrokers     string
	KafkaGroupID     string
	KafkaClientID    string
	TopicUserCreated string
	KafkaDLQTopic    string
	SMTPHost         string
	SMTPPort         string
	SMTPUsername     string
	SMTPPassword     string
	EmailFrom        string
	AllowedOrigins   []string
}

func Load() Config {
	return Config{
		Port:             env("PORT", "8083"),
		MongoURI:         env("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:          env("MONGO_DB", "notification_db"),
		JWTSecret:        env("JWT_SECRET", "your-secret-key"),
		KafkaBrokers:     env("KAFKA_BROKERS", ""),
		KafkaGroupID:     env("KAFKA_GROUP_ID", "notification-service-group"),
		KafkaClientID:    env("KAFKA_CLIENT_ID", "notification-service"),
		TopicUserCreated: env("KAFKA_TOPIC_USER_CREATED", "user.created.v1"),
		KafkaDLQTopic:    env("KAFKA_DLQ_TOPIC", "user.events.notification-service.dlq"),
		SMTPHost:         env("SMTP_HOST", ""),
		SMTPPort:         env("SMTP_PORT", "587"),
		SMTPUsername:     env("SMTP_USERNAME", ""),
		SMTPPassword:     env("SMTP_PASSWORD", ""),
		EmailFrom:        env("EMAIL_FROM", ""),
		AllowedOrigins:   splitCSV(env("ALLOWED_ORIGINS", "http://localhost:4200,http://localhost:8085")),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
