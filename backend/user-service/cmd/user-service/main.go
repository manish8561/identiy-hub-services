package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"user-service/internal/config"
	"user-service/internal/handlers"
	"user-service/internal/logger"
	"user-service/internal/router"
	"user-service/internal/services"

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

	// Initialize Kafka publisher for user lifecycle events.
	publisher, err := services.NewKafkaPublisher(
		cfg.KafkaBrokers,
		cfg.KafkaClientID,
		cfg.KafkaTopicUserCreated,
		cfg.KafkaTopicUserUpdated,
		cfg.KafkaTopicUserDeleted,
	)
	if err != nil {
		log.Error("Failed to initialize Kafka publisher", zap.Error(err))
	}
	defer func() {
		if publisher != nil {
			if closeErr := publisher.Close(); closeErr != nil {
				log.Error("Failed to close Kafka publisher", zap.Error(closeErr))
			}
		}
	}()

	// Initialize services
	userService := services.NewUserService(mongoConfig, publisher)
	var userServiceInterface services.UserServiceInterface = userService
	userHandler := handlers.NewUserHandler(userServiceInterface, log)
	log.Info("User service and handlers initialized")

	// Initialize Kafka consumer for user lifecycle events.
	consumer, err := services.NewUserEventConsumer(
		cfg.KafkaBrokers,
		cfg.KafkaGroupID,
		cfg.KafkaClientID,
		cfg.KafkaTopicUserCreated,
		cfg.KafkaTopicUserUpdated,
		cfg.KafkaTopicUserDeleted,
		userService,
		log,
	)
	if err != nil {
		log.Error("Failed to initialize Kafka consumer", zap.Error(err))
	}
	defer func() {
		if consumer != nil {
			if closeErr := consumer.Close(); closeErr != nil {
				log.Error("Failed to close Kafka consumer", zap.Error(closeErr))
			}
		}
	}()

	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()
	go func() {
		if consumer != nil {
			consumer.Start(consumerCtx)
		}
	}()

	// Setup routes using the router
	r := router.NewRouterWithAccountStatus(userHandler, log, cfg.JWTSecret, cfg.AllowedOrigins, userService.AccountActive)

	// Start the server
	serverAddr := fmt.Sprintf(":%s", cfg.Port)

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("User service starting", zap.String("port", cfg.Port))
		if err := r.Run(serverAddr); err != nil {
			log.Error("Failed to start user service", zap.Error(err))
		}
	}()

	<-quit
	cancelConsumer()
	log.Info("Shutting down user service...")
}
