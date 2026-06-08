package audit

import (
	"encoding/json"
	"time"
)

type LogEntry struct {
	ID         uint64          `json:"id"`
	UserID     *uint64         `json:"user_id,omitempty"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type,omitempty"`
	TargetID   string          `json:"target_id,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	IP         string          `json:"ip,omitempty"`
	UserAgent  string          `json:"user_agent,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

type ListQuery struct {
	Action string
	UserID *uint64
	Start  *time.Time
	End    *time.Time
}
