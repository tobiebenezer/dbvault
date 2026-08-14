package garbagecollection

import (
	"context"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Plan struct {
	ID               domain.GCPlanID
	RepositoryID     domain.RepositoryID
	RepositoryDigest string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	ReclaimableBytes int64
	DeleteKeys       []string
	Blocked          []string
}

type Service struct {
	Catalogue ports.Catalogue
	Store     ports.ObjectStore
	Clock     ports.Clock
}

func (s Service) Plan(ctx context.Context, repo domain.RepositoryID) (Plan, error) {
	now := s.Clock.Now()
	return Plan{ID: domain.GCPlanID("gc_" + now.Format("20060102150405")), RepositoryID: repo, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}
func (s Service) Run(ctx context.Context, plan Plan) error {
	if !plan.ExpiresAt.IsZero() && s.Clock.Now().After(plan.ExpiresAt) {
		return domain.NewError(domain.ErrGCPlanStale, "garbage-collection plan expired", nil)
	}
	for _, k := range plan.DeleteKeys {
		if err := s.Store.Delete(ctx, k); err != nil {
			return err
		}
	}
	return nil
}
