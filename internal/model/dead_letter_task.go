package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type DeadLetterTask struct {
	ID             uuid.UUID       `json:"id"`
	OriginalTaskID uuid.UUID       `json:"original_task_id"`
	Payload        json.RawMessage `json:"payload"`
	LastError      string          `json:"last_error"`
	AttemptCount   int             `json:"attempt_count"`
	FailedAt       time.Time       `json:"failed_at"`
	RetriedAt      *time.Time      `json:"retried_at"`
	CreatedAt      time.Time       `json:"created_at"`
}
