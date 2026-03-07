package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID           uuid.UUID       `json:"id"`
	Status       string          `json:"status"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	LockedBy     *string         `json:"locked_by"`
	LockedAt     *time.Time      `json:"locked_at"`
	AttemptCount int             `json:"attempt_count"`
	MaxRetries   int             `json:"max_retries"`
	LastError    *string         `json:"last_error"`
	NextRetryAt  *time.Time      `json:"next_retry_at"`
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
