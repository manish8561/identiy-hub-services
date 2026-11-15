package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"time"

	"auth-service/internal/models"
	"auth-service/internal/password"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	actionTokenCollection          = "auth_action_tokens"
	actionTokenEmailVerification   = "email_verification"
	actionTokenPasswordReset       = "password_reset"
	emailVerificationTokenLifetime = 24 * time.Hour
	passwordResetTokenLifetime     = 30 * time.Minute
)

type actionToken struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"userId"`
	Purpose   string    `bson:"purpose"`
	ExpiresAt time.Time `bson:"expiresAt"`
}

type EmailDeliveryError struct {
	Err error
}

func (e *EmailDeliveryError) Error() string {
	return fmt.Sprintf("deliver account email: %v", e.Err)
}

func (e *EmailDeliveryError) Unwrap() error {
	return e.Err
}

func (s *AuthService) EnsureActionTokenIndexes(ctx context.Context) error {
	collection := s.mongoConfig.GetCollection(actionTokenCollection)
	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0).SetName("action_token_expiry_ttl")},
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "purpose", Value: 1}}, Options: options.Index().SetName("action_token_user_purpose")},
	})
	if err != nil {
		return fmt.Errorf("create account action token indexes: %w", err)
	}
	return nil
}

func (s *AuthService) EnsureEmailVerificationDefaults(ctx context.Context) error {
	_, err := s.mongoConfig.GetCollection("auth_users").UpdateMany(
		ctx,
		bson.M{"emailVerified": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"emailVerified": true}},
	)
	if err != nil {
		return fmt.Errorf("mark legacy accounts as email verified: %w", err)
	}
	return nil
}

func (s *AuthService) VerifyEmail(token string) error {
	record, err := s.consumeActionToken(token, actionTokenEmailVerification)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	collection := s.mongoConfig.GetCollection("auth_users")
	var user models.User
	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": record.UserID, "status": bson.M{"$ne": "suspended"}},
		bson.M{"$set": bson.M{"emailVerified": true, "updatedAt": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrInvalidActionToken
		}
		return fmt.Errorf("verify account email: %w", err)
	}
	return s.publishUserUpdated(ctx, user)
}

func (s *AuthService) ResendVerification(email string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var user models.User
	err := s.mongoConfig.GetCollection("auth_users").FindOne(ctx, bson.M{
		"email": email, "emailVerified": false, "status": bson.M{"$ne": "suspended"},
	}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load account for email verification: %w", err)
	}
	return s.issueActionEmail(ctx, user, actionTokenEmailVerification)
}

func (s *AuthService) RequestPasswordReset(email string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var user models.User
	err := s.mongoConfig.GetCollection("auth_users").FindOne(ctx, bson.M{
		"email": email, "status": bson.M{"$ne": "suspended"},
	}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load account for password reset: %w", err)
	}
	return s.issueActionEmail(ctx, user, actionTokenPasswordReset)
}

func (s *AuthService) ResetPassword(token, newPassword string) error {
	if s.sessions == nil {
		return errors.New("user session store is not configured")
	}
	record, err := s.consumeActionToken(token, actionTokenPasswordReset)
	if err != nil {
		return err
	}
	hashedPassword, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash reset password: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := s.mongoConfig.GetCollection("auth_users").UpdateOne(ctx,
		bson.M{"_id": record.UserID, "status": bson.M{"$ne": "suspended"}},
		bson.M{"$set": bson.M{"password": hashedPassword, "updatedAt": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("update reset password: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrInvalidActionToken
	}
	if err := s.sessions.RevokeAll(ctx, record.UserID, time.Now()); err != nil {
		return &SessionStoreError{Operation: "revoke sessions after password reset", Err: err}
	}
	return nil
}

func (s *AuthService) SetAccountStatus(adminID, userID, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	users := s.mongoConfig.GetCollection("auth_users")

	var admin models.User
	if err := users.FindOne(ctx, bson.M{"_id": adminID}).Decode(&admin); err != nil {
		return fmt.Errorf("load administrator account: %w", err)
	}
	if admin.Role != "admin" {
		return ErrForbidden
	}
	if adminID == userID {
		return errors.New("administrators cannot change their own account status")
	}

	var user models.User
	if err := users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrInvalidActionToken
		}
		return fmt.Errorf("load account for status update: %w", err)
	}
	if user.Role != "customer" {
		return errors.New("only customer accounts can be suspended or reactivated")
	}
	if status != "active" && status != "suspended" {
		return errors.New("status must be active or suspended")
	}
	if user.Status == status {
		return s.publishUserUpdated(ctx, user)
	}
	if status == "suspended" && s.sessions == nil {
		return errors.New("user session store is not configured")
	}
	if status == "suspended" {
		if err := s.sessions.RevokeAll(ctx, userID, time.Now()); err != nil {
			return &SessionStoreError{Operation: "revoke suspended account sessions", Err: err}
		}
	}
	result, err := users.UpdateOne(ctx, bson.M{"_id": userID, "role": "customer"},
		bson.M{"$set": bson.M{"status": status, "updatedAt": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("update account status: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrInvalidActionToken
	}
	user.Status = status
	user.UpdatedAt = time.Now()
	return s.publishUserUpdated(ctx, user)
}

func (s *AuthService) UpdateUserByAdmin(adminID, userID string, request models.AdminUpdateUserRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	users := s.mongoConfig.GetCollection("auth_users")

	var admin models.User
	if err := users.FindOne(ctx, bson.M{"_id": adminID}).Decode(&admin); err != nil {
		return fmt.Errorf("load administrator account: %w", err)
	}
	if admin.Role != "admin" {
		return ErrForbidden
	}
	if adminID == userID {
		return errors.New("administrators cannot edit their own role through user management")
	}

	var user models.User
	if err := users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrInvalidActionToken
		}
		return fmt.Errorf("load account for update: %w", err)
	}
	if (user.Role != "customer" && user.Role != "admin") ||
		(request.Role != "customer" && request.Role != "admin") {
		return errors.New("account role is invalid")
	}
	if user.Role == "customer" && user.Status == "suspended" && request.Role == "admin" {
		return errors.New("reactivate the account before granting administrator access")
	}
	if user.Role == "admin" && request.Role == "customer" {
		otherAdminCount, err := users.CountDocuments(ctx, bson.M{
			"role": "admin", "status": bson.M{"$ne": "suspended"}, "_id": bson.M{"$ne": userID},
		})
		if err != nil {
			return fmt.Errorf("check remaining administrator accounts: %w", err)
		}
		if otherAdminCount == 0 {
			return errors.New("cannot demote the last active administrator")
		}
	}
	emailChanged := user.Email != request.Email
	roleChanged := user.Role != request.Role
	if emailChanged {
		duplicateCount, err := users.CountDocuments(ctx, bson.M{
			"email": request.Email, "_id": bson.M{"$ne": userID},
		})
		if err != nil {
			return fmt.Errorf("check account email: %w", err)
		}
		if duplicateCount > 0 {
			return errors.New("email address is already in use")
		}
	}
	if emailChanged || roleChanged {
		if s.sessions == nil {
			return errors.New("user session store is not configured")
		}
		if err := s.sessions.RevokeAll(ctx, userID, time.Now()); err != nil {
			return &SessionStoreError{Operation: "revoke sessions after account update", Err: err}
		}
	}
	if emailChanged {
		user.EmailVerified = false
	}

	updates := bson.M{
		"name": request.Name, "email": request.Email, "role": request.Role, "updatedAt": time.Now(),
	}
	if emailChanged {
		updates["emailVerified"] = false
	}
	result, err := users.UpdateOne(ctx, bson.M{"_id": userID, "role": user.Role},
		bson.M{"$set": updates},
	)
	if err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrInvalidActionToken
	}
	user.Name = request.Name
	user.Email = request.Email
	user.Role = request.Role
	user.UpdatedAt = time.Now()
	if err := s.publishUserUpdated(ctx, user); err != nil {
		return err
	}
	if emailChanged {
		return s.issueActionEmail(ctx, user, actionTokenEmailVerification)
	}
	return nil
}

func (s *AuthService) DeleteUserByAdmin(adminID, userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	users := s.mongoConfig.GetCollection("auth_users")

	var admin models.User
	if err := users.FindOne(ctx, bson.M{"_id": adminID}).Decode(&admin); err != nil {
		return fmt.Errorf("load administrator account: %w", err)
	}
	if admin.Role != "admin" {
		return ErrForbidden
	}
	if adminID == userID {
		return errors.New("administrators cannot delete their own account through user management")
	}
	var user models.User
	if err := users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrInvalidActionToken
		}
		return fmt.Errorf("load account for deletion: %w", err)
	}
	if user.Role != "customer" {
		return errors.New("only customer accounts can be deleted")
	}
	return s.deleteUserAccount(userID)
}

func (s *AuthService) deleteUserAccount(userID string) error {
	if s.publisher == nil {
		return errors.New("publish deletion event: publisher is not configured")
	}
	if s.sessions == nil {
		return errors.New("user session store is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.sessions.RevokeAll(ctx, userID, time.Now()); err != nil {
		return &SessionStoreError{Operation: "revoke account", Err: err}
	}

	var user models.User
	err := s.mongoConfig.GetCollection("auth_users").FindOneAndDelete(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("delete account: %w", err)
	}
	event := models.UserEvent{
		EventID: primitive.NewObjectID().Hex(), EventType: models.EventTypeUserDeleted,
		Timestamp: time.Now().UTC(), UserID: userID,
	}
	if user.ID != "" {
		event.Email, event.Name, event.Status, event.Role = user.Email, user.Name, user.Status, user.Role
	}
	if err := s.publisher.PublishUserDeleted(ctx, event); err != nil {
		return fmt.Errorf("account deleted but failed to publish deletion event: %w", err)
	}
	return nil
}

func (s *AuthService) UpdateOwnProfile(token, name, email, currentPassword string) (bool, error) {
	claims, active, err := s.validateActiveToken(token)
	if err != nil {
		return false, err
	}
	if !active {
		return false, ErrInvalidSession
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	users := s.mongoConfig.GetCollection("auth_users")
	var user models.User
	if err := users.FindOne(ctx, bson.M{"_id": claims.UserID}).Decode(&user); err != nil {
		return false, fmt.Errorf("load current account: %w", err)
	}

	emailChanged := user.Email != email
	if emailChanged {
		matches, _ := password.Check(user.Password, currentPassword)
		if !matches {
			return false, ErrInvalidCurrentPassword
		}
		duplicateCount, err := users.CountDocuments(ctx, bson.M{
			"email": email, "_id": bson.M{"$ne": user.ID},
		})
		if err != nil {
			return false, fmt.Errorf("check account email: %w", err)
		}
		if duplicateCount > 0 {
			return false, errors.New("email address is already in use")
		}
		if s.sessions == nil {
			return false, errors.New("user session store is not configured")
		}
		if err := s.sessions.RevokeAll(ctx, user.ID, time.Now()); err != nil {
			return false, &SessionStoreError{Operation: "revoke sessions after email change", Err: err}
		}
		user.EmailVerified = false
	}
	updates := bson.M{"name": name, "email": email, "updatedAt": time.Now()}
	if emailChanged {
		updates["emailVerified"] = false
	}
	result, err := users.UpdateOne(ctx, bson.M{"_id": user.ID},
		bson.M{"$set": updates},
	)
	if err != nil {
		return false, fmt.Errorf("update current account: %w", err)
	}
	if result.MatchedCount == 0 {
		return false, errors.New("account not found")
	}
	user.Name = name
	user.Email = email
	user.UpdatedAt = time.Now()
	if err := s.publishUserUpdated(ctx, user); err != nil {
		return emailChanged, err
	}
	if emailChanged {
		if err := s.issueActionEmail(ctx, user, actionTokenEmailVerification); err != nil {
			return true, fmt.Errorf("email changed but verification email failed: %w", err)
		}
	}
	return emailChanged, nil
}

func (s *AuthService) issueActionEmail(ctx context.Context, user models.User, purpose string) error {
	if s.emailSender == nil {
		return errors.New("transactional email sender is not configured")
	}
	rawToken, err := s.createActionToken(ctx, user.ID, purpose)
	if err != nil {
		return err
	}

	var subject, action, route string
	switch purpose {
	case actionTokenEmailVerification:
		subject, action, route = "Verify your email address", "verify your email address", "/verify-email"
	case actionTokenPasswordReset:
		subject, action, route = "Reset your password", "reset your password", "/reset-password"
	default:
		return errors.New("unsupported account email action")
	}
	link := s.frontendURL + route + "?token=" + url.QueryEscape(rawToken)
	body := fmt.Sprintf("Hello %s,\n\nUse the following link to %s:\n%s\n\nThis link expires soon. If you did not request this, you can ignore this email.\n", user.Name, action, link)
	if err := s.emailSender.Send(ctx, user.Email, subject, body); err != nil {
		return &EmailDeliveryError{Err: err}
	}
	return nil
}

func (s *AuthService) createActionToken(ctx context.Context, userID, purpose string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate account action token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expiresAt := time.Now().Add(emailVerificationTokenLifetime)
	if purpose == actionTokenPasswordReset {
		expiresAt = time.Now().Add(passwordResetTokenLifetime)
	}
	collection := s.mongoConfig.GetCollection(actionTokenCollection)
	if _, err := collection.DeleteMany(ctx, bson.M{"userId": userID, "purpose": purpose}); err != nil {
		return "", fmt.Errorf("replace existing account action token: %w", err)
	}
	_, err := collection.InsertOne(ctx, actionToken{
		ID:        base64.RawURLEncoding.EncodeToString(hash[:]),
		UserID:    userID,
		Purpose:   purpose,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("store account action token: %w", err)
	}
	return token, nil
}

func (s *AuthService) consumeActionToken(token, purpose string) (*actionToken, error) {
	hash := sha256.Sum256([]byte(token))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var record actionToken
	err := s.mongoConfig.GetCollection(actionTokenCollection).FindOneAndDelete(ctx, bson.M{
		"_id":       base64.RawURLEncoding.EncodeToString(hash[:]),
		"purpose":   purpose,
		"expiresAt": bson.M{"$gt": time.Now()},
	}).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrInvalidActionToken
	}
	if err != nil {
		return nil, fmt.Errorf("consume account action token: %w", err)
	}
	return &record, nil
}

func (s *AuthService) publishUserUpdated(ctx context.Context, user models.User) error {
	if s.publisher == nil {
		return errors.New("user lifecycle publisher is not configured")
	}
	event := models.UserEvent{
		EventID:       primitive.NewObjectID().Hex(),
		EventType:     models.EventTypeUserUpdated,
		Timestamp:     time.Now().UTC(),
		UserID:        user.ID,
		Email:         user.Email,
		Name:          user.Name,
		Status:        user.Status,
		Role:          user.Role,
		EmailVerified: boolPointer(user.EmailVerified),
	}
	if err := s.publisher.PublishUserUpdated(ctx, event); err != nil {
		return fmt.Errorf("publish account update: %w", err)
	}
	return nil
}

func boolPointer(value bool) *bool {
	return &value
}
