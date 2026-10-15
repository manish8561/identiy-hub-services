package services

import (
	"context"
	"testing"

	"auth-service/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitBrokersTrimsAndIgnoresEmptyEntries(t *testing.T) {
	assert.Equal(t, []string{"broker-1:9092", "broker-2:9092"}, splitBrokers(" broker-1:9092, ,broker-2:9092 "))
	assert.Empty(t, splitBrokers(" , , "))
}

func TestNewKafkaPublisher(t *testing.T) {
	publisher, err := NewKafkaPublisher("broker-1:9092, broker-2:9092", "auth-client", "user.created.v1", "user.updated.v1", "user.deleted.v1")
	require.NoError(t, err)
	require.NotNil(t, publisher)
	assert.Equal(t, "user.created.v1", publisher.topicUserCreated)
	assert.Equal(t, "user.updated.v1", publisher.topicUserUpdated)
	assert.Equal(t, "user.deleted.v1", publisher.topicUserDeleted)
	assert.NotNil(t, publisher.writer)

	publisher, err = NewKafkaPublisher(" ,  ", "auth-client", "user.created.v1", "user.updated.v1", "user.deleted.v1")
	require.NoError(t, err)
	assert.Nil(t, publisher)
}

func TestKafkaPublisherEmptyStateReturnsErrors(t *testing.T) {
	publisher := &KafkaPublisher{}
	event := models.UserEvent{UserID: "user-123", EventType: "user.created.v1"}

	assert.Error(t, publisher.PublishUserCreated(context.Background(), event))
	assert.Error(t, publisher.PublishUserUpdated(context.Background(), event))
	assert.Error(t, publisher.PublishUserDeleted(context.Background(), event))
	assert.NoError(t, publisher.Close())
}
