package models

import "time"

// User represents a user in the system
type User struct {
	ID        string    `json:"id" bson:"_id,omitempty"`
	Name      string    `json:"name" bson:"name"`
	Email     string    `json:"email" bson:"email"`
	Password  string    `json:"-" bson:"password"`
	Status    string    `json:"status" bson:"status"`
	Role      string    `json:"role" bson:"role"`
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

// UserListResponse represents a paginated list of users
type UserListResponse struct {
	Users      []User `json:"users"`
	Total      int64  `json:"total"`
	TotalCount int64  `json:"totalCount"`
	Page       int    `json:"page"`
	Size       int    `json:"size"`
}

// UpdateUserRequest represents a user update request
type UpdateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// CreateUserRequest represents a user creation request
type CreateUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}
