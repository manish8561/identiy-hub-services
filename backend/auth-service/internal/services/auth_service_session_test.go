package services_test

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"auth-service/internal/password"
	"auth-service/internal/services"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

const sessionTestJWTSecret = "session-test-secret-that-is-not-production"

func TestLoginPersistsSession(t *testing.T) {
	hashedPassword, err := password.Hash("correct-password")
	require.NoError(t, err)

	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("login persists a session and validates it", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "user-login"},
				{Key: "email", Value: "user@example.com"},
				{Key: "password", Value: hashedPassword},
			}),
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}},
			mtest.CreateCursorResponse(0, mt.DB.Name()+".user_sessions", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "session-login"},
			}),
		)

		jwtService := services.NewJWTService(config.NewJWTConfig(sessionTestJWTSecret))
		authService := services.NewAuthService(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			jwtService,
			nil,
		)

		response, err := authService.Login("user@example.com", "correct-password")
		require.NoError(t, err)
		validation, err := authService.ValidateToken(response.Token)
		require.NoError(t, err)
		require.True(t, validation.Valid)
		require.Equal(t, "user-login", validation.UserID)
	})
}

func TestRegisterPersistsSession(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("register persists a session", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
				{Key: "n", Value: 0},
			}),
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}},
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}},
			mtest.CreateCursorResponse(0, mt.DB.Name()+".user_sessions", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "session-register"},
			}),
		)

		jwtService := services.NewJWTService(config.NewJWTConfig(sessionTestJWTSecret))
		authService := services.NewAuthService(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			jwtService,
			nil,
		)

		response, err := authService.Register(models.RegisterRequest{
			Name:            "Registered User",
			Email:           "register@example.com",
			Password:        "correct-password",
			ConfirmPassword: "correct-password",
		})
		require.NoError(t, err)
		require.NotEmpty(t, response.Token)
		validation, err := authService.ValidateToken(response.Token)
		require.NoError(t, err)
		require.True(t, validation.Valid)
	})
}
