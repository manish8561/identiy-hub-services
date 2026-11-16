package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"notification-service/internal/models"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type EventConsumer struct {
	reader  *kafka.Reader
	dlq     *kafka.Writer
	service *NotificationService
	logger  *zap.Logger
}

const (
	maxProcessingAttempts = 5
	retryBackoff          = time.Second
)

func NewEventConsumer(brokers, groupID, clientID, topic, dlqTopic string, service *NotificationService, logger *zap.Logger) (*EventConsumer, error) {
	if strings.TrimSpace(brokers) == "" {
		return nil, nil
	}
	brokerList := make([]string, 0)
	for _, broker := range strings.Split(brokers, ",") {
		if broker = strings.TrimSpace(broker); broker != "" {
			brokerList = append(brokerList, broker)
		}
	}
	if len(brokerList) == 0 {
		return nil, nil
	}
	dlqTopic = strings.TrimSpace(dlqTopic)
	if groupID == "" || topic == "" || dlqTopic == "" {
		return nil, errors.New("kafka group id, user-created topic, and dead-letter topic are required")
	}
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: brokerList, GroupID: groupID, Topic: topic, MinBytes: 1, MaxBytes: 10e6, Dialer: &kafka.Dialer{ClientID: clientID}})
	dlq := &kafka.Writer{
		Addr:            kafka.TCP(brokerList...),
		Topic:           dlqTopic,
		Balancer:        &kafka.LeastBytes{},
		RequiredAcks:    kafka.RequireAll,
		Async:           false,
		MaxAttempts:     maxProcessingAttempts,
		WriteBackoffMin: 250 * time.Millisecond,
		WriteBackoffMax: 2 * time.Second,
		Transport:       &kafka.Transport{ClientID: clientID},
	}
	return &EventConsumer{reader: reader, dlq: dlq, service: service, logger: logger}, nil
}

func (c *EventConsumer) Start(ctx context.Context) {
	if c == nil || c.reader == nil {
		return
	}
	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			c.logger.Error("failed to fetch user-created event", zap.Error(err))
			if !waitForRetry(ctx, retryBackoff) {
				return
			}
			continue
		}
		attempts, processErr := retryMessage(ctx, maxProcessingAttempts, retryBackoff, func() error {
			var event models.UserEvent
			if err := json.Unmarshal(message.Value, &event); err != nil {
				return fmt.Errorf("unmarshal user-created event: %w", err)
			}
			return c.service.HandleUserCreated(ctx, event)
		})
		if processErr != nil {
			c.logger.Error("failed to persist notification outcome",
				zap.Error(processErr),
				zap.String("topic", message.Topic),
				zap.Int("attempts", attempts),
			)
			if errors.Is(processErr, context.Canceled) || ctx.Err() != nil {
				return
			}
			if err := c.publishDeadLetterWithRetry(ctx, message, attempts, processErr); err != nil {
				return
			}
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			c.logger.Error("failed to commit user-created event", zap.Error(err))
		}
	}
}

func retryMessage(ctx context.Context, maxAttempts int, initialBackoff time.Duration, process func() error) (int, error) {
	if maxAttempts < 1 {
		return 0, errors.New("max processing attempts must be positive")
	}
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return attempt - 1, ctx.Err()
		}
		if err = process(); err == nil {
			return attempt, nil
		}
		if attempt == maxAttempts {
			break
		}
		if !waitForRetry(ctx, retryDelay(initialBackoff, attempt)) {
			return attempt, ctx.Err()
		}
	}
	return maxAttempts, err
}

func retryDelay(initialBackoff time.Duration, attempt int) time.Duration {
	if initialBackoff <= 0 {
		return 0
	}
	delay := initialBackoff
	for remaining := attempt - 1; remaining > 0 && delay < 30*time.Second; remaining-- {
		if delay > 15*time.Second {
			return 30 * time.Second
		}
		delay *= 2
	}
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (c *EventConsumer) publishDeadLetterWithRetry(ctx context.Context, message kafka.Message, attempts int, cause error) error {
	for attempt := 1; ; attempt++ {
		if err := c.publishDeadLetter(ctx, message, attempts, cause); err == nil {
			return nil
		} else {
			c.logger.Error("failed to publish user-created event to dead-letter topic",
				zap.Error(err),
				zap.String("topic", message.Topic),
				zap.Int("processing_attempts", attempts),
				zap.Int("dead_letter_attempt", attempt),
			)
		}
		if !waitForRetry(ctx, retryDelay(retryBackoff, attempt)) {
			return ctx.Err()
		}
	}
}

func (c *EventConsumer) publishDeadLetter(ctx context.Context, message kafka.Message, attempts int, cause error) error {
	headers := append([]kafka.Header(nil), message.Headers...)
	headers = append(headers,
		kafka.Header{Key: "x-original-topic", Value: []byte(message.Topic)},
		kafka.Header{Key: "x-original-partition", Value: []byte(fmt.Sprint(message.Partition))},
		kafka.Header{Key: "x-original-offset", Value: []byte(fmt.Sprint(message.Offset))},
		kafka.Header{Key: "x-processing-attempts", Value: []byte(fmt.Sprint(attempts))},
		kafka.Header{Key: "x-processing-error", Value: []byte(cause.Error())},
	)
	return c.dlq.WriteMessages(ctx, kafka.Message{
		Key:     message.Key,
		Value:   message.Value,
		Headers: headers,
		Time:    time.Now().UTC(),
	})
}

func (c *EventConsumer) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	if c.reader != nil {
		closeErr = c.reader.Close()
	}
	if c.dlq != nil {
		closeErr = errors.Join(closeErr, c.dlq.Close())
	}
	return closeErr
}
