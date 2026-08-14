package ports

import (
	"context"
	"time"
)

type Event struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Severity  string            `json:"severity"`
	Resource  string            `json:"resource,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type Notifier interface {
	Notify(ctx context.Context, event Event) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}

type NoopNotifier struct{}

func (NoopNotifier) Notify(context.Context, Event) error  { return nil }
func (NoopNotifier) Publish(context.Context, Event) error { return nil }
