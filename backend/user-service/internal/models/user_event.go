package models

import "time"

const (
	EventTypeUserCreated = "user.created.v1"
	EventTypeUserUpdated = "user.updated.v1"
	EventTypeUserDeleted = "user.deleted.v1"
)

// UserEvent represents a user lifecycle event consumed from Kafka.
type UserEvent struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email,omitempty"`
	Name      string    `json:"name,omitempty"`
	Status    string    `json:"status,omitempty"`
	Role      string    `json:"role,omitempty"`
}
