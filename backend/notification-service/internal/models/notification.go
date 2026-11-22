package models

import "time"

type UserEvent struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	Timestamp     time.Time `json:"timestamp"`
	TraceID       string    `json:"trace_id,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	UserID        string    `json:"user_id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
}

type Preference struct {
	UserID       string    `json:"userId" bson:"_id"`
	EmailEnabled bool      `json:"emailEnabled" bson:"emailEnabled"`
	UpdatedAt    time.Time `json:"updatedAt" bson:"updatedAt"`
}

type Record struct {
	ID           string    `json:"id" bson:"_id"`
	EventID      string    `json:"-" bson:"eventId"`
	UserID       string    `json:"-" bson:"userId"`
	Type         string    `json:"type" bson:"type"`
	Channel      string    `json:"channel" bson:"channel"`
	Status       string    `json:"status" bson:"status"`
	CreatedAt    time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt" bson:"updatedAt"`
	ExpireAt     time.Time `json:"-" bson:"expireAt"`
	ErrorSummary string    `json:"errorSummary,omitempty" bson:"errorSummary,omitempty"`
}

type HistoryResponse struct {
	Items []Record `json:"items"`
	Total int64    `json:"total"`
	Page  int      `json:"page"`
	Size  int      `json:"size"`
}
