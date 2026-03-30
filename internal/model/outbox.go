package model

import (
	"encoding/json"
	"time"
)

type OutboxEvent struct {
	ID          int64           `json:"id"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
	PublishedAt *time.Time      `json:"published_at"`
}
