package services_test

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"auth-service/internal/password"
	"auth-service/internal/services"
	"context"
	"testing"
	"time"

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
				{Key: "emailVerified", Value: true},
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

func TestRegisterCreatesUnverifiedAccountAndSendsVerificationEmail(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("register persists a session", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
				{Key: "n", Value: 0},
			}),
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}},
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}},
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}},
		)

		jwtService := services.NewJWTService(config.NewJWTConfig(sessionTestJWTSecret))
		sender := &testEmailSender{}
		authService := services.NewAuthServiceWithDependencies(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			jwtService,
			nil,
			nil,
			sender,
			"https://app.example.test",
		)

		response, err := authService.Register(models.RegisterRequest{
			Name:            "Registered User",
			Email:           "register@example.com",
			Password:        "correct-password",
			ConfirmPassword: "correct-password",
		})
		require.NoError(t, err)
		require.Empty(t, response.Token)
		require.Contains(t, response.Message, "verify")
		require.Equal(t, "register@example.com", sender.recipient)
		require.Contains(t, sender.body, "https://app.example.test/verify-email?token=")
	})
}

type testEmailSender struct {
	recipient string
	subject   string
	body      string
}

func (s *testEmailSender) Send(_ context.Context, recipient, subject, body string) error {
	s.recipient = recipient
	s.subject = subject
	s.body = body
	return nil
}

func TestChangePasswordVerifiesCurrentPasswordAndRevokesOtherSessions(t *testing.T) {
	hashedPassword, err := password.Hash("current-password")
	require.NoError(t, err)

	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("changes password while retaining the current session", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
				{Key: "_id", Value: "user-password"},
				{Key: "password", Value: hashedPassword},
			}),
			bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}},
		)

		jwtService := services.NewJWTService(config.NewJWTConfig(sessionTestJWTSecret))
		sessions := newPasswordChangeSessionStore()
		token, sessionID, expiresAt, err := jwtService.GenerateTokenWithSession("user-password")
		require.NoError(t, err)
		require.NoError(t, sessions.Create(context.Background(), models.UserSession{
			ID: sessionID, UserID: "user-password", ExpiresAt: expiresAt,
		}))
		require.NoError(t, sessions.Create(context.Background(), models.UserSession{
			ID: "other-session", UserID: "user-password", ExpiresAt: time.Now().Add(time.Hour),
		}))

		authService := services.NewAuthServiceWithSessionStore(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			jwtService,
			nil,
			sessions,
		)
		err = authService.ChangePassword(token, "current-password", "replacement-password")
		require.NoError(t, err)

		currentActive, err := sessions.IsActive(context.Background(), sessionID, "user-password", time.Now())
		require.NoError(t, err)
		require.True(t, currentActive)
		otherActive, err := sessions.IsActive(context.Background(), "other-session", "user-password", time.Now())
		require.NoError(t, err)
		require.False(t, otherActive)
	})
}

func TestChangePasswordRejectsIncorrectCurrentPassword(t *testing.T) {
	hashedPassword, err := password.Hash("current-password")
	require.NoError(t, err)

	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("does not update password when current password is incorrect", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
			{Key: "_id", Value: "user-password"},
			{Key: "password", Value: hashedPassword},
		}))

		jwtService := services.NewJWTService(config.NewJWTConfig(sessionTestJWTSecret))
		sessions := newPasswordChangeSessionStore()
		token, sessionID, expiresAt, err := jwtService.GenerateTokenWithSession("user-password")
		require.NoError(t, err)
		require.NoError(t, sessions.Create(context.Background(), models.UserSession{
			ID: sessionID, UserID: "user-password", ExpiresAt: expiresAt,
		}))
		authService := services.NewAuthServiceWithSessionStore(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			jwtService,
			nil,
			sessions,
		)

		err = authService.ChangePassword(token, "wrong-password", "replacement-password")
		require.ErrorIs(t, err, services.ErrInvalidCurrentPassword)
	})
}

func TestLoginRejectsUnverifiedEmail(t *testing.T) {
	hashedPassword, err := password.Hash("correct-password")
	require.NoError(t, err)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("does not create a session before email verification", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".auth_users", mtest.FirstBatch, bson.D{
			{Key: "_id", Value: "user-unverified"},
			{Key: "email", Value: "user@example.com"},
			{Key: "password", Value: hashedPassword},
			{Key: "status", Value: "active"},
			{Key: "emailVerified", Value: false},
		}))
		authService := services.NewAuthService(
			config.NewMongoDBConfigFromDatabase(mt.DB),
			services.NewJWTService(config.NewJWTConfig(sessionTestJWTSecret)),
			nil,
		)

		_, err := authService.Login("user@example.com", "correct-password")
		require.ErrorIs(t, err, services.ErrEmailNotVerified)
	})
}

type passwordChangeSessionStore struct {
	sessions map[string]models.UserSession
}

func newPasswordChangeSessionStore() *passwordChangeSessionStore {
	return &passwordChangeSessionStore{sessions: make(map[string]models.UserSession)}
}

func (s *passwordChangeSessionStore) Create(_ context.Context, session models.UserSession) error {
	s.sessions[session.ID] = session
	return nil
}

func (s *passwordChangeSessionStore) IsActive(_ context.Context, sessionID, userID string, now time.Time) (bool, error) {
	session, ok := s.sessions[sessionID]
	return ok && session.UserID == userID && session.RevokedAt == nil && session.ExpiresAt.After(now), nil
}

func (s *passwordChangeSessionStore) Revoke(_ context.Context, sessionID, userID string, revokedAt time.Time) error {
	if session, ok := s.sessions[sessionID]; ok && session.UserID == userID && session.RevokedAt == nil {
		session.RevokedAt = &revokedAt
		s.sessions[sessionID] = session
	}
	return nil
}

func (s *passwordChangeSessionStore) RevokeAll(_ context.Context, userID string, revokedAt time.Time) error {
	return s.RevokeAllExcept(context.Background(), userID, "", revokedAt)
}

func (s *passwordChangeSessionStore) RevokeAllExcept(_ context.Context, userID, exceptSessionID string, revokedAt time.Time) error {
	for id, session := range s.sessions {
		if id != exceptSessionID && session.UserID == userID && session.RevokedAt == nil {
			session.RevokedAt = &revokedAt
			s.sessions[id] = session
		}
	}
	return nil
}
