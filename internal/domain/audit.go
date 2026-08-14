package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type AuditEventID string

type AuditEvent struct {
	ID           AuditEventID      `json:"id"`
	EventType    string            `json:"event_type"`
	ActorType    string            `json:"actor_type"`
	ActorID      string            `json:"actor_id,omitempty"`
	ResourceType string            `json:"resource_type,omitempty"`
	ResourceID   string            `json:"resource_id,omitempty"`
	Outcome      string            `json:"outcome"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	PreviousHash string            `json:"previous_hash,omitempty"`
	EventHash    string            `json:"event_hash"`
}

func ChainAuditEvent(e AuditEvent, previous string) AuditEvent {
	e.PreviousHash = previous
	e.EventHash = ""
	b, _ := json.Marshal(e)
	h := sha256.Sum256(b)
	e.EventHash = hex.EncodeToString(h[:])
	return e
}
