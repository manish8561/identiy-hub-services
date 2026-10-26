package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"auth-service/internal/config"
	"auth-service/internal/handlers"
	"auth-service/internal/logger"
	"auth-service/internal/router"
	"auth-service/internal/services"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	log, err := logger.NewZapLogger()
	if err != nil {
		log.Error("Failed to initialize logger", zap.Error(err))
	}
	defer log.Sync()

	// Load environment configuration
	cfg := config.LoadConfig()
	log.Info("Configuration loaded successfully")

	// Initialize MongoDB
	mongoConfig, err := config.NewMongoDBConfig(cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Error("Failed to connect to MongoDB", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		if closeErr := mongoConfig.Close(); closeErr != nil {
			log.Error("MongoDB close error", zap.Error(closeErr))
		}
	}()
	log.Info("MongoDB connection established")

	// Initialize JWT service
	jwtConfig := config.NewJWTConfig(cfg.JWTSecret)
	jwtService := services.NewJWTService(jwtConfig)
	log.Info("JWT service initialized")

	cookieConfig := config.NewCookieConfig(cfg.CookieSecure, cfg.CookieMaxAge, cfg.CookieSameSite, cfg.CookieDomain)
	sessionStore := services.NewMongoSessionStore(mongoConfig)
	indexCtx, cancelIndex := context.WithTimeout(context.Background(), 10*time.Second)
	if err := sessionStore.EnsureIndexes(indexCtx); err != nil {
		cancelIndex()
		log.Error("Failed to initialize user session indexes", zap.Error(err))
		os.Exit(1)
	}
	cancelIndex()
	log.Info("User session collection initialized")

	// Initialize Kafka publisher for user lifecycle events.
	kafkaPublisher, err := services.NewKafkaPublisher(
		cfg.KafkaBrokers,
		cfg.KafkaClientID,
		cfg.KafkaTopicUserCreated,
		cfg.KafkaTopicUserUpdated,
		cfg.KafkaTopicUserDeleted,
	)
	if err != nil {
		log.Error("Failed to initialize Kafka publisher", zap.Error(err))
	} else if kafkaPublisher == nil {
		log.Warn("Kafka publisher is not configured; account deletion is unavailable")
	}
	defer func() {
		if kafkaPublisher != nil {
			if closeErr := kafkaPublisher.Close(); closeErr != nil {
				log.Error("Failed to close Kafka publisher", zap.Error(closeErr))
			}
		}
	}()

	// Initialize services
	var userEventPublisher services.UserEventPublisher
	if kafkaPublisher != nil {
		userEventPublisher = kafkaPublisher
	}
	authService := services.NewAuthServiceWithSessionStore(mongoConfig, jwtService, userEventPublisher, sessionStore)
	authHandler := handlers.NewAuthHandler(authService, log, cookieConfig)
	log.Info("Auth service and handlers initialized")

	// Setup routes using the router
	r := router.NewRouter(authHandler, log, cfg.AllowedOrigins)

	// Start the server
	serverAddr := fmt.Sprintf(":%s", cfg.Port)

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("Auth service starting", zap.String("port", cfg.Port))
		if err := r.Run(serverAddr); err != nil {
			log.Error("Failed to start auth service", zap.Error(err))
		}
	}()

	<-quit
	log.Info("Shutting down auth service...")
}
