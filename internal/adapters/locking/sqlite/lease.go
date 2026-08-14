package sqlite

import (
	"context"
	"fmt"
	"os"
	"time"

	catsqlite "github.com/dbvault/dbvault/internal/adapters/catalogue/sqlite"
)

type Manager struct {
	Catalogue *catsqlite.Catalogue
	OwnerID   string
	Now       func() time.Time
}
type Lock struct {
	manager  *Manager
	resource string
	released bool
}

func New(cat *catsqlite.Catalogue, owner string) *Manager {
	if owner == "" {
		owner = fmt.Sprintf("pid-%d", os.Getpid())
	}
	return &Manager{Catalogue: cat, OwnerID: owner, Now: func() time.Time { return time.Now().UTC() }}
}
func (m *Manager) Acquire(ctx context.Context, resource string, ttl time.Duration) (*Lock, error) {
	now := m.Now()
	existing, ok, err := m.Catalogue.GetLease(ctx, resource)
	if err != nil {
		return nil, err
	}
	if ok && existing.ExpiresAt.After(now) && existing.OwnerID != m.OwnerID {
		return nil, fmt.Errorf("lease %s held by %s until %s", resource, existing.OwnerID, existing.ExpiresAt.Format(time.RFC3339))
	}
	rec := catsqlite.LeaseRecord{Resource: resource, OwnerID: m.OwnerID, AcquiredAt: now, HeartbeatAt: now, ExpiresAt: now.Add(ttl)}
	if err := m.Catalogue.UpsertLease(ctx, rec); err != nil {
		return nil, err
	}
	return &Lock{manager: m, resource: resource}, nil
}
func (l *Lock) Renew(ctx context.Context, ttl time.Duration) error {
	if l.released {
		return fmt.Errorf("lease released")
	}
	now := l.manager.Now()
	return l.manager.Catalogue.UpsertLease(ctx, catsqlite.LeaseRecord{Resource: l.resource, OwnerID: l.manager.OwnerID, AcquiredAt: now, HeartbeatAt: now, ExpiresAt: now.Add(ttl)})
}
func (l *Lock) Release(ctx context.Context) error {
	if l.released {
		return nil
	}
	l.released = true
	return l.manager.Catalogue.DeleteLease(ctx, l.resource)
}
