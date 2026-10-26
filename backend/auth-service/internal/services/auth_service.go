package services

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"auth-service/internal/password"
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// AuthService handles authentication-related business logic
type AuthService struct {
	mongoConfig *config.MongoDBConfig
	jwtService  *JWTService
	sessions    SessionStore
	publisher   UserEventPublisher
}

// UserEventPublisher publishes user lifecycle events.
type UserEventPublisher interface {
	PublishUserCreated(ctx context.Context, event models.UserEvent) error
	PublishUserUpdated(ctx context.Context, event models.UserEvent) error
	PublishUserDeleted(ctx context.Context, event models.UserEvent) error
}

// NewAuthService creates a new AuthService with the provided dependencies
func NewAuthService(mongoConfig *config.MongoDBConfig, jwtService *JWTService, publisher UserEventPublisher) *AuthService {
	return NewAuthServiceWithSessionStore(mongoConfig, jwtService, publisher, NewMongoSessionStore(mongoConfig))
}

// NewAuthServiceWithSessionStore creates an AuthService with an explicit session store.
func NewAuthServiceWithSessionStore(mongoConfig *config.MongoDBConfig, jwtService *JWTService, publisher UserEventPublisher, sessions SessionStore) *AuthService {
	return &AuthService{
		mongoConfig: mongoConfig,
		jwtService:  jwtService,
		sessions:    sessions,
		publisher:   publisher,
	}
}

// Login authenticates a user with the provided email and password.
// Returns a JWT token upon successful authentication or an error if credentials are invalid.
func (s *AuthService) Login(email, plainPassword string) (*models.LoginResponse, error) {
	collection := s.mongoConfig.GetCollection("auth_users")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := collection.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	ok, needsRehash := password.Check(user.Password, plainPassword)
	if !ok {
		return nil, errors.New("invalid credentials")
	}

	if needsRehash {
		hashed, hashErr := password.Hash(plainPassword)
		if hashErr == nil {
			_, _ = collection.UpdateOne(
				ctx,
				bson.M{"_id": user.ID},
				bson.M{"$set": bson.M{"password": hashed, "updatedAt": time.Now()}},
			)
		}
	}

	token, err := s.createSession(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("create login session: %w", err)
	}

	return &models.LoginResponse{
		Status: "success",
		Token:  token,
	}, nil
}

// Register creates a new user account with the provided registration details.
// Returns a success response if registration is successful, or an error if the user already exists.
func (s *AuthService) Register(req models.RegisterRequest) (*models.RegisterResponse, error) {
	collection := s.mongoConfig.GetCollection("auth_users")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if user already exists
	count, err := collection.CountDocuments(ctx, bson.M{"email": req.Email})
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("user already exists")
	}

	// Hash password
	hashedPassword, err := password.Hash(req.Password)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	// Create new user
	newUser := models.User{
		ID:        primitive.NewObjectID().Hex(),
		Name:      req.Name,
		Email:     req.Email,
		Password:  hashedPassword,
		Status:    "active",
		Role:      "customer",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Insert user into database
	_, err = collection.InsertOne(ctx, newUser)
	if err != nil {
		return nil, err
	}

	event := models.UserEvent{
		EventID:   primitive.NewObjectID().Hex(),
		EventType: models.EventTypeUserCreated,
		Timestamp: time.Now().UTC(),
		UserID:    newUser.ID,
		Email:     newUser.Email,
		Name:      newUser.Name,
		Status:    newUser.Status,
		Role:      newUser.Role,
	}

	if s.publisher != nil {
		if err := s.publisher.PublishUserCreated(ctx, event); err != nil {
			// Registration should succeed even if async sync event publishing fails.
			// Retry/reconciliation can handle eventual consistency.
		}
	}

	token, err := s.createSession(ctx, newUser.ID)
	if err != nil {
		return nil, fmt.Errorf("create registration session: %w", err)
	}

	return &models.RegisterResponse{
		Status:  "success",
		Message: "User registered successfully",
		Token:   token,
	}, nil
}

// DeleteAccount removes an account and publishes its deletion event.
// If publishing fails after the account is deleted, a subsequent call with the
// same user ID republishes the event, allowing clients to retry propagation.
func (s *AuthService) DeleteAccount(userID string) error {
	if s.publisher == nil {
		return errors.New("publish deletion event: publisher is not configured")
	}
	if s.sessions == nil {
		return errors.New("user session store is not configured")
	}

	collection := s.mongoConfig.GetCollection("auth_users")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.sessions.RevokeAll(ctx, userID, time.Now()); err != nil {
		return &SessionStoreError{Operation: "revoke account", Err: err}
	}

	var user models.User
	err := collection.FindOneAndDelete(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("delete account: %w", err)
	}

	event := models.UserEvent{
		EventID:   primitive.NewObjectID().Hex(),
		EventType: models.EventTypeUserDeleted,
		Timestamp: time.Now().UTC(),
		UserID:    userID,
	}
	if user.ID != "" {
		event.Email = user.Email
		event.Name = user.Name
		event.Status = user.Status
		event.Role = user.Role
	}
	if err := s.publisher.PublishUserDeleted(ctx, event); err != nil {
		return fmt.Errorf("account deleted but failed to publish deletion event: %w", err)
	}

	return nil
}

// ValidateToken validates a JWT token and returns user information
func (s *AuthService) ValidateToken(tokenString string) (*models.TokenValidationResponse, error) {
	claims, active, err := s.validateActiveToken(tokenString)
	if err != nil {
		return nil, err
	}
	if !active {
		return &models.TokenValidationResponse{Valid: false, Message: "Invalid token"}, nil
	}

	return &models.TokenValidationResponse{
		Valid:  true,
		UserID: claims.UserID,
	}, nil
}

// RefreshToken generates a new token for the user
func (s *AuthService) RefreshToken(tokenString string) (*models.RefreshTokenResponse, error) {
	claims, active, err := s.validateActiveToken(tokenString)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, errors.New("invalid token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	newToken, err := s.createSession(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("create refreshed session: %w", err)
	}
	if err := s.sessions.Revoke(ctx, claims.ID, claims.UserID, time.Now()); err != nil {
		return nil, &SessionStoreError{Operation: "revoke refreshed", Err: err}
	}

	return &models.RefreshTokenResponse{
		Status: "success",
		Token:  newToken,
	}, nil
}

// Logout revokes the session represented by tokenString. Invalid or missing tokens
// are treated as already logged out so logout remains safe to retry.
func (s *AuthService) Logout(tokenString string) error {
	claims, err := s.jwtService.ValidateToken(tokenString)
	if err != nil || claims.ID == "" {
		return nil
	}
	if s.sessions == nil {
		return &SessionStoreError{Operation: "logout", Err: errors.New("user session store is not configured")}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.sessions.Revoke(ctx, claims.ID, claims.UserID, time.Now()); err != nil {
		return &SessionStoreError{Operation: "logout", Err: err}
	}
	return nil
}

func (s *AuthService) createSession(ctx context.Context, userID string) (string, error) {
	if s.sessions == nil {
		return "", &SessionStoreError{Operation: "create", Err: errors.New("user session store is not configured")}
	}

	token, sessionID, expiresAt, err := s.jwtService.GenerateTokenWithSession(userID)
	if err != nil {
		return "", err
	}
	if err := s.sessions.Create(ctx, models.UserSession{
		ID:        sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}); err != nil {
		return "", &SessionStoreError{Operation: "create", Err: err}
	}
	return token, nil
}

func (s *AuthService) validateActiveToken(tokenString string) (*Claims, bool, error) {
	claims, err := s.jwtService.ValidateToken(tokenString)
	if err != nil || claims.ID == "" {
		return nil, false, nil
	}
	if s.sessions == nil {
		return nil, false, &SessionStoreError{Operation: "validate", Err: errors.New("user session store is not configured")}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	active, err := s.sessions.IsActive(ctx, claims.ID, claims.UserID, time.Now())
	if err != nil {
		return nil, false, &SessionStoreError{Operation: "validate", Err: err}
	}
	return claims, active, nil
}
