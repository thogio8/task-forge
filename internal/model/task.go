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
