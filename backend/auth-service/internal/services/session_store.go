package services

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const userSessionsCollection = "user_sessions"

// SessionStore persists issued sessions so authentication can be revoked server-side.
type SessionStore interface {
	Create(ctx context.Context, session models.UserSession) error
	IsActive(ctx context.Context, sessionID, userID string, now time.Time) (bool, error)
	Revoke(ctx context.Context, sessionID, userID string, revokedAt time.Time) error
	RevokeAll(ctx context.Context, userID string, revokedAt time.Time) error
}

// SessionStoreError identifies a failure that prevented a session persistence operation.
type SessionStoreError struct {
	Operation string
	Err       error
}

func (e *SessionStoreError) Error() string {
	return fmt.Sprintf("%s user session: %v", e.Operation, e.Err)
}

func (e *SessionStoreError) Unwrap() error {
	return e.Err
}

// MongoSessionStore stores one document for each issued authentication cookie.
type MongoSessionStore struct {
	collection *mongo.Collection
}

func NewMongoSessionStore(mongoConfig *config.MongoDBConfig) *MongoSessionStore {
	if mongoConfig == nil {
		return nil
	}
	return &MongoSessionStore{collection: mongoConfig.GetCollection(userSessionsCollection)}
}

func (s *MongoSessionStore) EnsureIndexes(ctx context.Context) error {
	if s == nil || s.collection == nil {
		return fmt.Errorf("initialize user session indexes: MongoDB collection is not configured")
	}

	_, err := s.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("session_expiry_ttl"),
		},
		{
			Keys:    bson.D{{Key: "userId", Value: 1}},
			Options: options.Index().SetName("session_user_id"),
		},
	})
	if err != nil {
		return fmt.Errorf("create user session indexes: %w", err)
	}
	return nil
}

func (s *MongoSessionStore) Create(ctx context.Context, session models.UserSession) error {
	if s == nil || s.collection == nil {
		return fmt.Errorf("create user session: MongoDB collection is not configured")
	}
	if _, err := s.collection.InsertOne(ctx, session); err != nil {
		return fmt.Errorf("create user session: %w", err)
	}
	return nil
}

func (s *MongoSessionStore) IsActive(ctx context.Context, sessionID, userID string, now time.Time) (bool, error) {
	if s == nil || s.collection == nil {
		return false, fmt.Errorf("validate user session: MongoDB collection is not configured")
	}

	err := s.collection.FindOne(ctx, bson.M{
		"_id":       sessionID,
		"userId":    userID,
		"expiresAt": bson.M{"$gt": now},
		"revokedAt": bson.M{"$exists": false},
	}).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("validate user session: %w", err)
	}
	return true, nil
}

func (s *MongoSessionStore) Revoke(ctx context.Context, sessionID, userID string, revokedAt time.Time) error {
	if s == nil || s.collection == nil {
		return fmt.Errorf("revoke user session: MongoDB collection is not configured")
	}
	_, err := s.collection.UpdateOne(ctx, bson.M{
		"_id":       sessionID,
		"userId":    userID,
		"revokedAt": bson.M{"$exists": false},
	}, bson.M{"$set": bson.M{"revokedAt": revokedAt}})
	if err != nil {
		return fmt.Errorf("revoke user session: %w", err)
	}
	return nil
}

func (s *MongoSessionStore) RevokeAll(ctx context.Context, userID string, revokedAt time.Time) error {
	if s == nil || s.collection == nil {
		return fmt.Errorf("revoke user sessions: MongoDB collection is not configured")
	}
	_, err := s.collection.UpdateMany(ctx, bson.M{
		"userId":    userID,
		"revokedAt": bson.M{"$exists": false},
	}, bson.M{"$set": bson.M{"revokedAt": revokedAt}})
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}
