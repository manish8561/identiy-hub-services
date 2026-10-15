package services

import (
	"auth-service/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/segmentio/kafka-go"
)

// KafkaPublisher publishes user lifecycle events.
type KafkaPublisher struct {
	writer           *kafka.Writer
	topicUserCreated string
	topicUserUpdated string
	topicUserDeleted string
}

// NewKafkaPublisher creates a publisher for user events.
func NewKafkaPublisher(brokers, clientID, createdTopic, updatedTopic, deletedTopic string) (*KafkaPublisher, error) {
	parsedBrokers := splitBrokers(brokers)
	if len(parsedBrokers) == 0 {
		return nil, nil
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(parsedBrokers...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
		Transport: &kafka.Transport{
			ClientID: clientID,
		},
	}

	return &KafkaPublisher{
		writer:           writer,
		topicUserCreated: createdTopic,
		topicUserUpdated: updatedTopic,
		topicUserDeleted: deletedTopic,
	}, nil
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

func (p *KafkaPublisher) publish(ctx context.Context, topic string, event models.UserEvent) error {
	if p == nil || p.writer == nil {
		return fmt.Errorf("publish user event: Kafka publisher is not configured")
	}
	if topic == "" {
		return fmt.Errorf("publish user event: Kafka topic is not configured")
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(event.UserID),
		Value: payload,
	})
}

// PublishUserCreated publishes user.created.v1.
func (p *KafkaPublisher) PublishUserCreated(ctx context.Context, event models.UserEvent) error {
	return p.publish(ctx, p.topicUserCreated, event)
}

// PublishUserUpdated publishes user.updated.v1.
func (p *KafkaPublisher) PublishUserUpdated(ctx context.Context, event models.UserEvent) error {
	return p.publish(ctx, p.topicUserUpdated, event)
}

// PublishUserDeleted publishes user.deleted.v1.
func (p *KafkaPublisher) PublishUserDeleted(ctx context.Context, event models.UserEvent) error {
	return p.publish(ctx, p.topicUserDeleted, event)
}

// Close closes the underlying writer.
func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("close kafka writer: %w", err)
	}
	return nil
}
