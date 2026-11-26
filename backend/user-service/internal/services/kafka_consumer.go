package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"user-service/internal/logger"
	"user-service/internal/models"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// UserEventConsumer consumes user lifecycle events and updates user profiles.
type UserEventConsumer struct {
	logger           logger.Logger
	service          *UserService
	readers          []*kafka.Reader
	dlqWriter        *kafka.Writer
	topicUserCreated string
	topicUserUpdated string
	topicUserDeleted string
}

const (
	maxProcessingAttempts = 5
	retryBackoff          = time.Second
)

// NewUserEventConsumer creates a Kafka consumer for user lifecycle topics.
func NewUserEventConsumer(
	brokers string,
	groupID string,
	clientID string,
	topicUserCreated string,
	topicUserUpdated string,
	topicUserDeleted string,
	dlqTopic string,
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
	dlqTopic = strings.TrimSpace(dlqTopic)
	if dlqTopic == "" {
		return nil, errors.New("kafka dead-letter topic is required")
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
	dlqWriter := &kafka.Writer{
		Addr:            kafka.TCP(parsedBrokers...),
		Topic:           dlqTopic,
		Balancer:        &kafka.LeastBytes{},
		RequiredAcks:    kafka.RequireAll,
		Async:           false,
		MaxAttempts:     maxProcessingAttempts,
		WriteBackoffMin: 250 * time.Millisecond,
		WriteBackoffMax: 2 * time.Second,
		Transport: &kafka.Transport{
			ClientID: clientID,
		},
	}

	return &UserEventConsumer{
		logger:           log,
		service:          service,
		readers:          readers,
		dlqWriter:        dlqWriter,
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
			if !waitForRetry(ctx, retryBackoff) {
				return
			}
			continue
		}

		carrier := propagation.MapCarrier{}
		for _, header := range msg.Headers {
			carrier[strings.ToLower(header.Key)] = string(header.Value)
		}
		processCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
		processCtx, span := otel.Tracer("user-service/kafka").Start(processCtx, "process "+msg.Topic, oteltrace.WithSpanKind(oteltrace.SpanKindConsumer))

		attempts, processErr := retryMessage(processCtx, maxProcessingAttempts, retryBackoff, func() error {
			var event models.UserEvent
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				return fmt.Errorf("unmarshal user event: %w", err)
			}
			return c.handleEvent(msg.Topic, event)
		})
		if processErr != nil {
			span.RecordError(processErr)
			span.SetStatus(codes.Error, processErr.Error())
		}
		if processErr != nil {
			traceID := kafkaHeaderValue(msg.Headers, "x-trace-id")
			correlationID := kafkaHeaderValue(msg.Headers, "x-correlation-id")
			if traceID == "" || correlationID == "" {
				var event models.UserEvent
				if json.Unmarshal(msg.Value, &event) == nil {
					if traceID == "" {
						traceID = event.TraceID
					}
					if correlationID == "" {
						correlationID = event.CorrelationID
					}
				}
			}
			c.logger.Error("Failed to process user event",
				zap.Error(processErr),
				zap.String("topic", msg.Topic),
				zap.Int("attempts", attempts),
				zap.String("trace_id", traceID),
				zap.String("correlation_id", correlationID),
			)
			if errors.Is(processErr, context.Canceled) || ctx.Err() != nil {
				span.End()
				return
			}
			if err := c.publishDeadLetterWithRetry(ctx, msg, attempts, processErr); err != nil {
				span.End()
				return
			}
		}
		if processErr == nil {
			traceID := kafkaHeaderValue(msg.Headers, "x-trace-id")
			correlationID := kafkaHeaderValue(msg.Headers, "x-correlation-id")
			var event models.UserEvent
			if json.Unmarshal(msg.Value, &event) == nil {
				if traceID == "" {
					traceID = event.TraceID
				}
				if correlationID == "" {
					correlationID = event.CorrelationID
				}
			}
			c.logger.Info("Processed user event",
				zap.String("topic", msg.Topic),
				zap.String("trace_id", traceID),
				zap.String("correlation_id", correlationID),
			)
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("Failed to commit Kafka message", zap.Error(err))
		}
		span.End()
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

func (c *UserEventConsumer) publishDeadLetterWithRetry(ctx context.Context, message kafka.Message, attempts int, cause error) error {
	for attempt := 1; ; attempt++ {
		if err := c.publishDeadLetter(ctx, message, attempts, cause); err == nil {
			return nil
		} else {
			c.logger.Error("Failed to publish user event to dead-letter topic",
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

func (c *UserEventConsumer) publishDeadLetter(ctx context.Context, message kafka.Message, attempts int, cause error) error {
	headers := append([]kafka.Header(nil), message.Headers...)
	if kafkaHeaderValue(headers, "x-trace-id") == "" {
		var event models.UserEvent
		if json.Unmarshal(message.Value, &event) == nil && event.TraceID != "" {
			headers = append(headers, kafka.Header{Key: "x-trace-id", Value: []byte(event.TraceID)})
		}
	}
	if kafkaHeaderValue(headers, "x-correlation-id") == "" {
		var event models.UserEvent
		if json.Unmarshal(message.Value, &event) == nil && event.CorrelationID != "" {
			headers = append(headers, kafka.Header{Key: "x-correlation-id", Value: []byte(event.CorrelationID)})
		}
	}
	headers = append(headers,
		kafka.Header{Key: "x-original-topic", Value: []byte(message.Topic)},
		kafka.Header{Key: "x-original-partition", Value: []byte(fmt.Sprint(message.Partition))},
		kafka.Header{Key: "x-original-offset", Value: []byte(fmt.Sprint(message.Offset))},
		kafka.Header{Key: "x-processing-attempts", Value: []byte(fmt.Sprint(attempts))},
		kafka.Header{Key: "x-processing-error", Value: []byte(cause.Error())},
	)
	return c.dlqWriter.WriteMessages(ctx, kafka.Message{
		Key:     message.Key,
		Value:   message.Value,
		Headers: headers,
		Time:    time.Now().UTC(),
	})
}

func kafkaHeaderValue(headers []kafka.Header, key string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Key, key) {
			return string(header.Value)
		}
	}
	return ""
}

func (c *UserEventConsumer) handleEvent(topic string, event models.UserEvent) error {
	if strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.UserID) == "" {
		return errors.New("user event is missing its event ID or user ID")
	}
	switch topic {
	case c.topicUserCreated:
		if event.EventType != models.EventTypeUserCreated {
			return fmt.Errorf("unexpected event type %q on topic %q", event.EventType, topic)
		}
		return c.service.UpsertUserProfileFromEvent(event)
	case c.topicUserUpdated:
		if event.EventType != models.EventTypeUserUpdated {
			return fmt.Errorf("unexpected event type %q on topic %q", event.EventType, topic)
		}
		return c.service.UpsertUserProfileFromEvent(event)
	case c.topicUserDeleted:
		if event.EventType != models.EventTypeUserDeleted {
			return fmt.Errorf("unexpected event type %q on topic %q", event.EventType, topic)
		}
		return c.service.DeleteUserProfileFromEvent(event)
	default:
		switch event.EventType {
		case models.EventTypeUserCreated:
			return c.service.UpsertUserProfileFromEvent(event)
		case models.EventTypeUserUpdated:
			return c.service.UpsertUserProfileFromEvent(event)
		case models.EventTypeUserDeleted:
			return c.service.DeleteUserProfileFromEvent(event)
		default:
			return fmt.Errorf("unknown user event type %q", event.EventType)
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
	if c.dlqWriter != nil {
		if err := c.dlqWriter.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
			c.logger.Error("Failed to close Kafka dead-letter writer", zap.Error(err))
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
