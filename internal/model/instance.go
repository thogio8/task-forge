package model

import "time"

type Instance struct {
	ID              string    `json:"id"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	StartedAt       time.Time `json:"started_at"`
}
