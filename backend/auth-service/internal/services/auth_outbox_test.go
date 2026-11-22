package services

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestRegistrationUserDocumentContainsPendingEvent(t *testing.T) {
	event := models.UserEvent{
		EventID: "event-1", EventType: models.EventTypeUserCreated, UserID: "user-1",
		TraceID: "0123456789abcdef0123456789abcdef", CorrelationID: "correlation-1",
	}
	document, err := registrationUserDocument(models.User{ID: "user-1", Email: "user@example.test"}, event)
	require.NoError(t, err)

	var storedEvent models.UserEvent
	require.NoError(t, json.Unmarshal([]byte(document[pendingLifecycleEventField].(string)), &storedEvent))
	require.Equal(t, event, storedEvent)
	require.Equal(t, "user-1", document["_id"])
}

func TestPendingRegistrationEventRemainsAfterPublishFailure(t *testing.T) {
	event := models.UserEvent{EventID: "event-1", EventType: models.EventTypeUserCreated, UserID: "user-1"}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("retains outbox event when Kafka publication fails", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
			{Key: "_id", Value: "user-1"},
			{Key: pendingLifecycleEventField, Value: string(payload)},
		}))
		publisher := &pendingEventPublisher{err: errors.New("broker unavailable")}
		service := NewAuthServiceWithDependencies(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			nil,
			publisher,
			nil,
			nil,
			"",
		)

		err := service.PublishPendingUserCreatedEvents(context.Background())
		require.ErrorContains(t, err, "broker unavailable")
		require.Equal(t, 1, publisher.createdCalls)
	})
}

type pendingEventPublisher struct {
	err          error
	createdCalls int
}

func (p *pendingEventPublisher) PublishUserCreated(context.Context, models.UserEvent) error {
	p.createdCalls++
	return p.err
}

func (*pendingEventPublisher) PublishUserUpdated(context.Context, models.UserEvent) error { return nil }
func (*pendingEventPublisher) PublishUserDeleted(context.Context, models.UserEvent) error { return nil }
