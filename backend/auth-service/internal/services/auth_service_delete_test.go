package services_test

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"auth-service/internal/password"
	"auth-service/internal/services"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestDeleteAccountRequiresPublisher(t *testing.T) {
	authService := services.NewAuthService(nil, nil, nil)

	if err := authService.DeleteAccount("token", "current-password"); err == nil {
		t.Fatal("DeleteAccount() error = nil, want missing publisher error")
	}
}

func TestDeleteAccountVerifiesPasswordRevokesSessionsAndPublishesDeletion(t *testing.T) {
	hashedPassword, err := password.Hash("current-password")
	require.NoError(t, err)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	mt.Run("deletes only after password confirmation", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "user-123"},
				{Key: "name", Value: "Test User"},
				{Key: "email", Value: "test@example.com"},
				{Key: "password", Value: hashedPassword},
				{Key: "status", Value: "active"},
				{Key: "role", Value: "customer"},
			}),
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "value", Value: bson.D{
					{Key: "_id", Value: "user-123"},
					{Key: "name", Value: "Test User"},
					{Key: "email", Value: "test@example.com"},
					{Key: "status", Value: "active"},
					{Key: "role", Value: "customer"},
					{Key: "password", Value: hashedPassword},
				}},
			},
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
		})).Return(nil).Once()

		jwtService := services.NewJWTService(config.NewJWTConfig(sessionTestJWTSecret))
		sessions := newPasswordChangeSessionStore()
		token, sessionID, expiresAt, err := jwtService.GenerateTokenWithSession("user-123")
		require.NoError(t, err)
		require.NoError(t, sessions.Create(context.Background(), models.UserSession{
			ID: sessionID, UserID: "user-123", ExpiresAt: expiresAt,
		}))
		authService := services.NewAuthServiceWithSessionStore(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			jwtService,
			publisher,
			sessions,
		)

		require.NoError(t, authService.DeleteAccount(token, "current-password"))
		active, err := sessions.IsActive(context.Background(), sessionID, "user-123", time.Now())
		require.NoError(t, err)
		require.False(t, active)
		publisher.AssertExpectations(t)
	})
}
