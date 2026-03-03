package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID        uuid.UUID       `json:"id"`
	Status    string          `json:"status"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

var validStatuses = map[string]bool{
	StatusPending:   true,
	StatusRunning:   true,
	StatusCompleted: true,
	StatusFailed:    true,
}

func IsValidStatus(s string) bool {
	return validStatuses[s]
}
