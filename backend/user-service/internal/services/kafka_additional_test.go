package services

import (
	"context"
	"testing"
	"user-service/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubLogger struct{}

func (stubLogger) Info(string, ...zap.Field)  {}
func (stubLogger) Warn(string, ...zap.Field)  {}
func (stubLogger) Error(string, ...zap.Field) {}
func (stubLogger) Sync() error                { return nil }

func TestSplitBrokersTrimsEmptyBrokerEntries(t *testing.T) {
	assert.Equal(t, []string{"broker-1:9092", "broker-2:9092"}, splitBrokers(" broker-1:9092, ,broker-2:9092 "))
	assert.Empty(t, splitBrokers(" , , "))
}

func TestNewUserEventConsumer(t *testing.T) {
	consumer, err := NewUserEventConsumer(
		"broker-1:9092, broker-2:9092",
		"user-group",
		"user-client",
		"user.created.v1",
		"user.updated.v1",
		"user.deleted.v1",
		"user.events.user-service.dlq",
		&UserService{},
		stubLogger{},
	)
	require.NoError(t, err)
	require.NotNil(t, consumer)
	assert.Len(t, consumer.readers, 3)
	assert.Equal(t, "user.created.v1", consumer.topicUserCreated)
	assert.Equal(t, "user.updated.v1", consumer.topicUserUpdated)
	assert.Equal(t, "user.deleted.v1", consumer.topicUserDeleted)
	require.NotNil(t, consumer.dlqWriter)

	consumer, err = NewUserEventConsumer("broker-1:9092", "", "user-client", "user.created.v1", "user.updated.v1", "user.deleted.v1", "user.events.user-service.dlq", &UserService{}, stubLogger{})
	assert.ErrorContains(t, err, "kafka group id is required")
	assert.Nil(t, consumer)

	consumer, err = NewUserEventConsumer("broker-1:9092", "user-group", "user-client", "user.created.v1", "user.updated.v1", "user.deleted.v1", "", &UserService{}, stubLogger{})
	assert.ErrorContains(t, err, "dead-letter topic is required")
	assert.Nil(t, consumer)

	consumer, err = NewUserEventConsumer(" , , ", "user-group", "user-client", "user.created.v1", "user.updated.v1", "user.deleted.v1", "user.events.user-service.dlq", &UserService{}, stubLogger{})
	require.NoError(t, err)
	assert.Nil(t, consumer)
}

func TestRetryMessageUsesBoundedAttempts(t *testing.T) {
	attempts := 0
	gotAttempts, err := retryMessage(context.Background(), 5, 0, func() error {
		attempts++
		if attempts < 3 {
			return assert.AnError
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, gotAttempts)
	assert.Equal(t, 3, attempts)

	attempts = 0
	gotAttempts, err = retryMessage(context.Background(), 5, 0, func() error {
		attempts++
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 5, gotAttempts)
	assert.Equal(t, 5, attempts)
}

func TestHandleEventRejectsMalformedEvent(t *testing.T) {
	consumer := &UserEventConsumer{topicUserCreated: "user.created.v1"}
	err := consumer.handleEvent("user.created.v1", models.UserEvent{EventType: models.EventTypeUserCreated})
	assert.ErrorContains(t, err, "missing its event ID or user ID")
}

func TestUserEventConsumerNoopsForEmptyState(t *testing.T) {
	var consumer *UserEventConsumer
	assert.NoError(t, consumer.Close())
	consumer = &UserEventConsumer{}
	assert.NoError(t, consumer.Close())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	consumer = &UserEventConsumer{}
	consumer.Start(ctx)
}
