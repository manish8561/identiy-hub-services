package services_test

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"auth-service/internal/services"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestVerifyEmailConsumesTokenAndPublishesUpdatedProfile(t *testing.T) {
	rawToken := "verification-token"
	tokenHash := sha256.Sum256([]byte(rawToken))
	tokenID := base64.RawURLEncoding.EncodeToString(tokenHash[:])
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("verifies the address using a stored token hash", func(mt *mtest.T) {
		mt.AddMockResponses(
			bson.D{{Key: "ok", Value: 1}, {Key: "value", Value: bson.D{
				{Key: "_id", Value: tokenID},
				{Key: "userId", Value: "user-verify"},
				{Key: "purpose", Value: "email_verification"},
				{Key: "expiresAt", Value: time.Now().Add(time.Hour)},
			}}},
			bson.D{{Key: "ok", Value: 1}, {Key: "value", Value: bson.D{
				{Key: "_id", Value: "user-verify"},
				{Key: "name", Value: "Verified User"},
				{Key: "email", Value: "verified@example.com"},
				{Key: "status", Value: "active"},
				{Key: "role", Value: "customer"},
				{Key: "emailVerified", Value: true},
			}}},
		)

		publisher := new(MockKafkaPublisher)
		publisher.On("PublishUserUpdated", mock.Anything, mock.MatchedBy(func(event models.UserEvent) bool {
			return event.UserID == "user-verify" &&
				event.EventType == models.EventTypeUserUpdated &&
				event.EmailVerified != nil && *event.EmailVerified
		})).Return(nil).Once()
		authService := services.NewAuthService(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			nil,
			publisher,
		)

		require.NoError(t, authService.VerifyEmail(rawToken))
		publisher.AssertExpectations(t)
	})
}

func TestVerifyEmailRejectsExpiredOrConsumedToken(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("does not accept a missing token record", func(mt *mtest.T) {
		mt.AddMockResponses(bson.D{{Key: "ok", Value: 1}, {Key: "value", Value: nil}})
		authService := services.NewAuthService(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			nil,
			nil,
		)

		require.ErrorIs(t, authService.VerifyEmail("not-valid"), services.ErrInvalidActionToken)
	})
}

func TestResetPasswordRevokesAllSessions(t *testing.T) {
	rawToken := "password-reset-token"
	tokenHash := sha256.Sum256([]byte(rawToken))
	tokenID := base64.RawURLEncoding.EncodeToString(tokenHash[:])
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("changes password and revokes existing sessions", func(mt *mtest.T) {
		mt.AddMockResponses(
			bson.D{{Key: "ok", Value: 1}, {Key: "value", Value: bson.D{
				{Key: "_id", Value: tokenID},
				{Key: "userId", Value: "user-reset"},
				{Key: "purpose", Value: "password_reset"},
				{Key: "expiresAt", Value: time.Now().Add(time.Hour)},
			}}},
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}},
		)

		sessions := newPasswordChangeSessionStore()
		require.NoError(t, sessions.Create(context.Background(), models.UserSession{
			ID: "session-reset", UserID: "user-reset", ExpiresAt: time.Now().Add(time.Hour),
		}))
		authService := services.NewAuthServiceWithSessionStore(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			nil,
			nil,
			sessions,
		)

		require.NoError(t, authService.ResetPassword(rawToken, "new-password"))
		active, err := sessions.IsActive(context.Background(), "session-reset", "user-reset", time.Now())
		require.NoError(t, err)
		require.False(t, active)
	})
}

func TestAdminCanPromoteCustomerAndPublishRoleChange(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("persists role in auth storage and publishes profile sync", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "admin-1"},
				{Key: "role", Value: "admin"},
			}),
			mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "customer-1"},
				{Key: "name", Value: "Customer"},
				{Key: "email", Value: "customer@example.com"},
				{Key: "status", Value: "active"},
				{Key: "role", Value: "customer"},
				{Key: "emailVerified", Value: true},
			}),
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}},
		)
		publisher := new(MockKafkaPublisher)
		publisher.On("PublishUserUpdated", mock.Anything, mock.MatchedBy(func(event models.UserEvent) bool {
			return event.UserID == "customer-1" && event.Role == "admin"
		})).Return(nil).Once()
		authService := services.NewAuthServiceWithSessionStore(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			nil,
			publisher,
			newPasswordChangeSessionStore(),
		)

		err := authService.UpdateUserByAdmin("admin-1", "customer-1", models.AdminUpdateUserRequest{
			Name: "Customer", Email: "customer@example.com", Role: "admin",
		})
		require.NoError(t, err)
		publisher.AssertExpectations(t)
	})
}
