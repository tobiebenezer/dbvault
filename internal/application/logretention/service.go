package logretention

import (
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type LogGCPlanID string

type LogGCPlan struct {
	ID               LogGCPlanID
	SourceID         domain.SourceID
	RepositoryID     domain.RepositoryID
	CatalogueVersion int64
	RecoveryDigest   string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	Candidates       []domain.TransactionLogID
	EstimatedBytes   int64
}

type Planner struct {
	Now   func() time.Time
	Grace time.Duration
}

func (p Planner) Plan(source domain.SourceID, repo domain.RepositoryID, logs []domain.TransactionLog, required map[domain.TransactionLogID]bool) LogGCPlan {
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	plan := LogGCPlan{ID: LogGCPlanID("loggc_" + now.Format("20060102150405")), SourceID: source, RepositoryID: repo, CreatedAt: now, ExpiresAt: now.Add(time.Hour), RecoveryDigest: "uncomputed"}
	grace := p.Grace
	if grace == 0 {
		grace = 7 * 24 * time.Hour
	}
	for _, l := range logs {
		if required[l.ID] || l.Status == domain.LogVerified || now.Sub(l.CollectedAt) < grace {
			continue
		}
		plan.Candidates = append(plan.Candidates, l.ID)
		plan.EstimatedBytes += l.StoredSize
	}
	return plan
}
