package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"user-service/internal/logger"
	"user-service/internal/models"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// UserEventConsumer consumes user lifecycle events and updates user profiles.
type UserEventConsumer struct {
	logger           logger.Logger
	service          *UserService
	readers          []*kafka.Reader
	topicUserCreated string
	topicUserUpdated string
	topicUserDeleted string
}

// NewUserEventConsumer creates a Kafka consumer for user lifecycle topics.
func NewUserEventConsumer(
	brokers string,
	groupID string,
	clientID string,
	topicUserCreated string,
	topicUserUpdated string,
	topicUserDeleted string,
	service *UserService,
	log logger.Logger,
) (*UserEventConsumer, error) {
	parsedBrokers := splitBrokers(brokers)
	if len(parsedBrokers) == 0 {
		return nil, nil
	}

	if groupID == "" {
		return nil, errors.New("kafka group id is required")
	}

	topics := []string{topicUserCreated, topicUserUpdated, topicUserDeleted}
	readers := make([]*kafka.Reader, 0, len(topics))

	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}

		readers = append(readers, kafka.NewReader(kafka.ReaderConfig{
			Brokers:  parsedBrokers,
			GroupID:  groupID,
			Topic:    topic,
			MinBytes: 1,
			MaxBytes: 10e6,
			Dialer: &kafka.Dialer{
				ClientID: clientID,
			},
		}))
	}

	if len(readers) == 0 {
		return nil, nil
	}

	return &UserEventConsumer{
		logger:           log,
		service:          service,
		readers:          readers,
		topicUserCreated: strings.TrimSpace(topicUserCreated),
		topicUserUpdated: strings.TrimSpace(topicUserUpdated),
		topicUserDeleted: strings.TrimSpace(topicUserDeleted),
	}, nil
}

// Start begins consuming all configured topics until context cancellation.
func (c *UserEventConsumer) Start(ctx context.Context) {
	if c == nil || len(c.readers) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(c.readers))

	for _, reader := range c.readers {
		go func(r *kafka.Reader) {
			defer wg.Done()
			c.consumeLoop(ctx, r)
		}(reader)
	}

	wg.Wait()
}

func (c *UserEventConsumer) consumeLoop(ctx context.Context, reader *kafka.Reader) {
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			c.logger.Error("Failed to fetch Kafka message", zap.Error(err))
			continue
		}

		var event models.UserEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			c.logger.Error("Failed to unmarshal user event",
				zap.Error(err),
				zap.String("topic", msg.Topic),
			)
			if commitErr := reader.CommitMessages(ctx, msg); commitErr != nil {
				c.logger.Error("Failed to commit malformed message", zap.Error(commitErr))
			}
			continue
		}

		if err := c.handleEvent(msg.Topic, event); err != nil {
			c.logger.Error("Failed to process user event",
				zap.Error(err),
				zap.String("topic", msg.Topic),
				zap.String("event_type", event.EventType),
				zap.String("user_id", event.UserID),
			)
			continue
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("Failed to commit Kafka message", zap.Error(err))
		}
	}
}

func (c *UserEventConsumer) handleEvent(topic string, event models.UserEvent) error {
	switch topic {
	case c.topicUserCreated, c.topicUserUpdated:
		return c.service.UpsertUserProfileFromEvent(event)
	case c.topicUserDeleted:
		return c.service.DeleteUserProfileFromEvent(event)
	default:
		switch event.EventType {
		case models.EventTypeUserCreated, models.EventTypeUserUpdated:
			return c.service.UpsertUserProfileFromEvent(event)
		case models.EventTypeUserDeleted:
			return c.service.DeleteUserProfileFromEvent(event)
		default:
			c.logger.Warn("Ignoring unknown user event type", zap.String("event_type", event.EventType))
			return nil
		}
	}
}

// Close closes all reader resources.
func (c *UserEventConsumer) Close() error {
	if c == nil {
		return nil
	}

	var closeErr error
	for _, reader := range c.readers {
		if err := reader.Close(); err != nil {
			closeErr = err
			c.logger.Error("Failed to close Kafka reader", zap.Error(err))
		}
	}
	return closeErr
}

func splitBrokers(brokers string) []string {
	raw := strings.Split(brokers, ",")
	parsed := make([]string, 0, len(raw))
	for _, broker := range raw {
		broker = strings.TrimSpace(broker)
		if broker != "" {
			parsed = append(parsed, broker)
		}
	}
	return parsed
}
