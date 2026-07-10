package models

import (
	"encoding/json"
	"time"
)

type OutboxMessage struct {
	ID        string          `json:"id"`
	Topic     string          `json:"topic"`
	Tier      int             `json:"tier"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
