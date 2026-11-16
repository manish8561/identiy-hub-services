package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"notification-service/internal/config"
	"notification-service/internal/handlers"
	"notification-service/internal/router"
	"notification-service/internal/services"

	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	mongo, err := config.ConnectMongo(cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		logger.Fatal("failed to connect to MongoDB", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongo.Close(ctx); err != nil {
			logger.Error("failed to close MongoDB", zap.Error(err))
		}
	}()

	repository, err := services.NewMongoRepository(mongo.Database())
	if err != nil {
		logger.Fatal("failed to initialize notification collections", zap.Error(err))
	}
	email := services.NewSMTPEmailSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.EmailFrom)
	notificationService := services.NewNotificationService(repository, email)
	handler := handlers.NewNotificationHandler(notificationService)
	consumer, err := services.NewEventConsumer(cfg.KafkaBrokers, cfg.KafkaGroupID, cfg.KafkaClientID, cfg.TopicUserCreated, cfg.KafkaDLQTopic, notificationService, logger)
	if err != nil {
		logger.Fatal("failed to initialize Kafka consumer", zap.Error(err))
	}
	if consumer != nil {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		go consumer.Start(ctx)
		defer consumer.Close()
	}

	server := router.New(handler, cfg.JWTSecret, cfg.AllowedOrigins)
	logger.Info("notification service starting", zap.String("port", cfg.Port))
	if err := server.Run(fmt.Sprintf(":%s", cfg.Port)); err != nil {
		logger.Fatal("notification service stopped", zap.Error(err))
	}
}
