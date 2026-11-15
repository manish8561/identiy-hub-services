package services_test

import (
	"testing"
	"time"
	"user-service/internal/config"
	"user-service/internal/services"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestAccountActiveRequiresVerifiedProfileAndActiveSession(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("allows active verified user with an active session", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".user_profiles", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "user-active"},
				{Key: "status", Value: "active"},
				{Key: "emailVerified", Value: true},
			}),
			mtest.CreateCursorResponse(0, "auth_db.user_sessions", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "session-active"},
				{Key: "userId", Value: "user-active"},
				{Key: "expiresAt", Value: time.Now().Add(time.Hour)},
			}),
		)
		userService := services.NewUserService(config.NewMongoDBConfigFromDatabase(mt.DB), nil)

		active, err := userService.AccountActive("user-active", "session-active")
		require.NoError(t, err)
		require.True(t, active)
	})
}

func TestAccountActiveRejectsMissingSession(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("rejects a user without a stored session", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".user_profiles", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "user-active"},
				{Key: "status", Value: "active"},
				{Key: "emailVerified", Value: true},
			}),
			mtest.CreateCursorResponse(0, "auth_db.user_sessions", mtest.FirstBatch),
		)
		userService := services.NewUserService(config.NewMongoDBConfigFromDatabase(mt.DB), nil)

		active, err := userService.AccountActive("user-active", "revoked-session")
		require.NoError(t, err)
		require.False(t, active)
	})
}
