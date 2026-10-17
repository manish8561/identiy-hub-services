package services

import (
	"context"
	"errors"
	"time"
	"user-service/internal/config"
	"user-service/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UserService handles user-related business logic
type UserService struct {
	mongoConfig *config.MongoDBConfig
	publisher   *KafkaPublisher
}

// NewUserService creates a new UserService with the provided MongoDB configuration
func NewUserService(mongoConfig *config.MongoDBConfig, publisher *KafkaPublisher) *UserService {
	return &UserService{
		mongoConfig: mongoConfig,
		publisher:   publisher,
	}
}

// GetUserByID retrieves a user by their unique identifier
func (s *UserService) GetUserByID(id string) (*models.User, error) {
	collection := s.mongoConfig.GetCollection("user_profiles")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if err != nil {
		return nil, errors.New("user not found")
	}

	return &user, nil
}

// ListUsers returns a paginated list of users and the total count
func (s *UserService) ListUsers(page, pageSize int) (*models.UserListResponse, error) {
	collection := s.mongoConfig.GetCollection("user_profiles")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	filter := bson.M{"role": "customer"}
	findOptions := options.Find().SetSkip(skip).SetLimit(limit)
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	for cursor.Next(ctx) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &models.UserListResponse{
		Users:      users,
		Total:      total,
		TotalCount: total,
		Page:       page,
		Size:       pageSize,
	}, nil
}

// UpdateUser updates a user's information
func (s *UserService) UpdateUser(id string, req models.UpdateUserRequest) (*models.User, error) {
	collection := s.mongoConfig.GetCollection("user_profiles")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"name":      req.Name,
			"email":     req.Email,
			"role":      req.Role,
			"updatedAt": time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return nil, err
	}

	if result.MatchedCount == 0 {
		return nil, errors.New("user not found")
	}

	updatedUser, err := s.GetUserByID(id)
	if err != nil {
		return nil, err
	}

	event := models.UserEvent{
		EventID:   primitive.NewObjectID().Hex(),
		EventType: models.EventTypeUserUpdated,
		Timestamp: time.Now().UTC(),
		UserID:    updatedUser.ID,
		Email:     updatedUser.Email,
		Name:      updatedUser.Name,
		Status:    updatedUser.Status,
		Role:      updatedUser.Role,
	}
	if err := s.publisher.PublishUserUpdated(ctx, event); err != nil {
		// Keep API behavior successful even if async event publishing fails.
	}

	return updatedUser, nil
}

// DeleteUser deletes a user by ID
func (s *UserService) DeleteUser(id string) error {
	collection := s.mongoConfig.GetCollection("user_profiles")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return errors.New("user not found")
	}

	event := models.UserEvent{
		EventID:   primitive.NewObjectID().Hex(),
		EventType: models.EventTypeUserDeleted,
		Timestamp: time.Now().UTC(),
		UserID:    id,
	}
	if err := s.publisher.PublishUserDeleted(ctx, event); err != nil {
		// Keep API behavior successful even if async event publishing fails.
	}

	return nil
}

// UpsertUserProfileFromEvent creates or updates a profile from a user lifecycle event.
func (s *UserService) UpsertUserProfileFromEvent(event models.UserEvent) error {
	collection := s.mongoConfig.GetCollection("user_profiles")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"name":      event.Name,
			"email":     event.Email,
			"status":    event.Status,
			"role":      event.Role,
			"updatedAt": time.Now(),
		},
		"$setOnInsert": bson.M{
			"_id":       event.UserID,
			"createdAt": time.Now(),
		},
	}

	_, err := collection.UpdateOne(ctx, bson.M{"_id": event.UserID}, update, options.Update().SetUpsert(true))
	return err
}

// DeleteUserProfileFromEvent removes a profile by user ID using event payload.
func (s *UserService) DeleteUserProfileFromEvent(event models.UserEvent) error {
	collection := s.mongoConfig.GetCollection("user_profiles")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.DeleteOne(ctx, bson.M{"_id": event.UserID})
	return err
}

// CreateUser creates a new user
func (s *UserService) CreateUser(req models.CreateUserRequest) (*models.User, error) {
	collection := s.mongoConfig.GetCollection("user_profiles")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user := models.User{
		ID:        primitive.NewObjectID().Hex(),
		Name:      req.Name,
		Email:     req.Email,
		Password:  req.Password,
		Role:      req.Role,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, user)
	if err != nil {
		return nil, err
	}

	event := models.UserEvent{
		EventID:   primitive.NewObjectID().Hex(),
		EventType: models.EventTypeUserCreated,
		Timestamp: time.Now().UTC(),
		UserID:    user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Status:    user.Status,
		Role:      user.Role,
	}
	if err := s.publisher.PublishUserCreated(ctx, event); err != nil {
		// Keep API behavior successful even if async event publishing fails.
	}

	return &user, nil
}
