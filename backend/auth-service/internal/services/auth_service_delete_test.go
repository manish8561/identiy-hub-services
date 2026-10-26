package services_test

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"auth-service/internal/services"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestDeleteAccountRequiresPublisher(t *testing.T) {
	authService := services.NewAuthService(nil, nil, nil)

	if err := authService.DeleteAccount("user-123"); err == nil {
		t.Fatal("DeleteAccount() error = nil, want missing publisher error")
	}
}

func TestDeleteAccountPublishesDeletionAndCanRetryAfterPublishFailure(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	mt.Run("retry publishes deletion event", func(mt *mtest.T) {
		mt.AddMockResponses(
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 0}, {Key: "nModified", Value: 0}},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "value", Value: bson.D{
					{Key: "_id", Value: "user-123"},
					{Key: "name", Value: "Test User"},
					{Key: "email", Value: "test@example.com"},
					{Key: "status", Value: "active"},
					{Key: "role", Value: "customer"},
				}},
			},
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 0}, {Key: "nModified", Value: 0}},
			bson.D{{Key: "ok", Value: 1}, {Key: "value", Value: nil}},
		)

		publisher := new(MockKafkaPublisher)
		publisher.On("PublishUserDeleted", mock.Anything, mock.MatchedBy(func(event models.UserEvent) bool {
			return event.EventType == "user.deleted.v1" &&
				event.UserID == "user-123" &&
				event.Email == "test@example.com" &&
				event.Name == "Test User" &&
				event.Status == "active" &&
				event.Role == "customer" &&
				!event.Timestamp.IsZero()
		})).Return(errors.New("kafka unavailable")).Once()
		publisher.On("PublishUserDeleted", mock.Anything, mock.MatchedBy(func(event models.UserEvent) bool {
			return event.EventType == "user.deleted.v1" &&
				event.UserID == "user-123" &&
				event.Email == "" &&
				!event.Timestamp.Before(time.Now().Add(-time.Minute))
		})).Return(nil).Once()

		authService := services.NewAuthService(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			nil,
			publisher,
		)

		if err := authService.DeleteAccount("user-123"); err == nil {
			t.Fatal("DeleteAccount() error = nil, want publish failure")
		}
		if err := authService.DeleteAccount("user-123"); err != nil {
			t.Fatalf("DeleteAccount() retry error = %v", err)
		}
		publisher.AssertExpectations(t)
	})
}
