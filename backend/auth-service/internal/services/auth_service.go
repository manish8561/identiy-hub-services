package services

import (
	"auth-service/internal/config"
	"auth-service/internal/models"
	"auth-service/internal/password"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AuthService handles authentication-related business logic
type AuthService struct {
	mongoConfig *config.MongoDBConfig
	jwtService  *JWTService
	sessions    SessionStore
	publisher   UserEventPublisher
	emailSender EmailSender
	frontendURL string
}

var (
	ErrInvalidCurrentPassword = errors.New("current password is incorrect")
	ErrInvalidSession         = errors.New("invalid session")
	ErrPasswordUnchanged      = errors.New("new password must be different from the current password")
	ErrPasswordTooLong        = errors.New("new password exceeds the maximum supported length")
	ErrEmailNotVerified       = errors.New("email address is not verified")
	ErrAccountSuspended       = errors.New("account is suspended")
	ErrInvalidActionToken     = errors.New("invalid or expired action token")
	ErrForbidden              = errors.New("forbidden")
)

const pendingLifecycleEventField = "pendingLifecycleEvent"

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
	return NewAuthServiceWithDependencies(mongoConfig, jwtService, publisher, sessions, nil, "")
}

func NewAuthServiceWithDependencies(
	mongoConfig *config.MongoDBConfig,
	jwtService *JWTService,
	publisher UserEventPublisher,
	sessions SessionStore,
	emailSender EmailSender,
	frontendURL string,
) *AuthService {
	return &AuthService{
		mongoConfig: mongoConfig,
		jwtService:  jwtService,
		sessions:    sessions,
		publisher:   publisher,
		emailSender: emailSender,
		frontendURL: frontendURL,
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
	if user.Status == "suspended" {
		return nil, ErrAccountSuspended
	}
	// Legacy accounts predate the emailVerified field and remain verified.
	if !user.EmailVerified {
		var legacyUser bson.M
		if err := collection.FindOne(ctx, bson.M{
			"_id":           user.ID,
			"emailVerified": bson.M{"$exists": false},
		}).Decode(&legacyUser); err != nil {
			return nil, ErrEmailNotVerified
		}
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

// ChangePassword verifies the active session and current password before replacing
// the stored password. Other active sessions are revoked while this session remains valid.
func (s *AuthService) ChangePassword(tokenString, currentPassword, newPassword string) error {
	if len([]byte(newPassword)) > 72 {
		return ErrPasswordTooLong
	}

	claims, active, err := s.validateActiveToken(tokenString)
	if err != nil {
		return err
	}
	if !active {
		return ErrInvalidSession
	}

	collection := s.mongoConfig.GetCollection("auth_users")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": claims.UserID}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrInvalidCurrentPassword
		}
		return fmt.Errorf("load user for password change: %w", err)
	}

	matches, _ := password.Check(user.Password, currentPassword)
	if !matches {
		return ErrInvalidCurrentPassword
	}
	if currentPassword == newPassword {
		return ErrPasswordUnchanged
	}

	hashedPassword, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	result, err := collection.UpdateOne(ctx, bson.M{
		"_id":      user.ID,
		"password": user.Password,
	}, bson.M{"$set": bson.M{
		"password":  hashedPassword,
		"updatedAt": time.Now(),
	}})
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrInvalidCurrentPassword
	}
	if err := s.sessions.RevokeAllExcept(ctx, claims.UserID, claims.ID, time.Now()); err != nil {
		return &SessionStoreError{Operation: "revoke other sessions after password change", Err: err}
	}

	return nil
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
		ID:            primitive.NewObjectID().Hex(),
		Name:          req.Name,
		Email:         req.Email,
		Password:      hashedPassword,
		Status:        "active",
		Role:          "customer",
		EmailVerified: false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	event := models.UserEvent{
		EventID:       primitive.NewObjectID().Hex(),
		EventType:     models.EventTypeUserCreated,
		Timestamp:     time.Now().UTC(),
		UserID:        newUser.ID,
		Email:         newUser.Email,
		Name:          newUser.Name,
		Status:        newUser.Status,
		Role:          newUser.Role,
		EmailVerified: boolPointer(newUser.EmailVerified),
	}
	registrationDocument, err := registrationUserDocument(newUser, event)
	if err != nil {
		return nil, err
	}
	// Store the event with the account so publication can be reconciled after process or broker failure.
	_, err = collection.InsertOne(ctx, registrationDocument)
	if err != nil {
		return nil, err
	}

	if err := s.issueActionEmail(ctx, newUser, actionTokenEmailVerification); err != nil {
		return nil, fmt.Errorf("send verification email: %w", err)
	}

	return &models.RegisterResponse{
		Status:  "success",
		Message: "Registration successful. Check your email to verify your account.",
	}, nil
}

func registrationUserDocument(user models.User, event models.UserEvent) (bson.M, error) {
	eventPayload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("encode registration event: %w", err)
	}
	return bson.M{
		"_id":                      user.ID,
		"name":                     user.Name,
		"email":                    user.Email,
		"password":                 user.Password,
		"status":                   user.Status,
		"role":                     user.Role,
		"emailVerified":            user.EmailVerified,
		"createdAt":                user.CreatedAt,
		"updatedAt":                user.UpdatedAt,
		pendingLifecycleEventField: string(eventPayload),
	}, nil
}

// EnsurePendingLifecycleEventIndex keeps reconciliation scans bounded to pending records.
func (s *AuthService) EnsurePendingLifecycleEventIndex(ctx context.Context) error {
	_, err := s.mongoConfig.GetCollection("auth_users").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: pendingLifecycleEventField, Value: 1}},
		Options: options.Index().SetSparse(true).SetName("pending_lifecycle_event"),
	})
	if err != nil {
		return fmt.Errorf("create pending lifecycle event index: %w", err)
	}
	return nil
}

// PublishPendingUserCreatedEvents retries durable registration events and clears them only
// after Kafka acknowledges publication.
func (s *AuthService) PublishPendingUserCreatedEvents(ctx context.Context) error {
	if s.publisher == nil {
		return errors.New("user lifecycle publisher is not configured")
	}
	users := s.mongoConfig.GetCollection("auth_users")
	cursor, err := users.Find(ctx, bson.M{
		pendingLifecycleEventField: bson.M{"$exists": true},
	})
	if err != nil {
		return fmt.Errorf("find pending user lifecycle events: %w", err)
	}
	defer cursor.Close(ctx)

	var reconciliationErr error
	for cursor.Next(ctx) {
		var record struct {
			ID      string `bson:"_id"`
			Payload string `bson:"pendingLifecycleEvent"`
		}
		if err := cursor.Decode(&record); err != nil {
			reconciliationErr = errors.Join(reconciliationErr, fmt.Errorf("decode pending lifecycle event: %w", err))
			continue
		}

		var event models.UserEvent
		if err := json.Unmarshal([]byte(record.Payload), &event); err != nil {
			reconciliationErr = errors.Join(reconciliationErr, fmt.Errorf("decode pending lifecycle event for user %s: %w", record.ID, err))
			continue
		}
		if event.EventType != models.EventTypeUserCreated {
			reconciliationErr = errors.Join(reconciliationErr, fmt.Errorf("unsupported pending lifecycle event type %q for user %s", event.EventType, record.ID))
			continue
		}
		if err := s.publisher.PublishUserCreated(ctx, event); err != nil {
			reconciliationErr = errors.Join(reconciliationErr, fmt.Errorf("publish pending registration event for user %s: %w", record.ID, err))
			continue
		}
		if _, err := users.UpdateOne(ctx,
			bson.M{"_id": record.ID, pendingLifecycleEventField: record.Payload},
			bson.M{"$unset": bson.M{pendingLifecycleEventField: ""}},
		); err != nil {
			reconciliationErr = errors.Join(reconciliationErr, fmt.Errorf("clear published registration event for user %s: %w", record.ID, err))
		}
	}
	if err := cursor.Err(); err != nil {
		reconciliationErr = errors.Join(reconciliationErr, fmt.Errorf("iterate pending lifecycle events: %w", err))
	}
	return reconciliationErr
}

// DeleteAccount removes the authenticated account and publishes its deletion event.
func (s *AuthService) DeleteAccount(tokenString, currentPassword string) error {
	if s.publisher == nil {
		return errors.New("publish deletion event: publisher is not configured")
	}
	if s.sessions == nil {
		return errors.New("user session store is not configured")
	}
	claims, active, err := s.validateActiveToken(tokenString)
	if err != nil {
		return err
	}
	if !active {
		return ErrInvalidSession
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := s.mongoConfig.GetCollection("auth_users").FindOne(ctx, bson.M{"_id": claims.UserID}).Decode(&user); err != nil {
		return fmt.Errorf("load account for deletion: %w", err)
	}
	matches, _ := password.Check(user.Password, currentPassword)
	if !matches {
		return ErrInvalidCurrentPassword
	}
	return s.deleteUserAccount(claims.UserID)
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
