package recovery

import (
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Planner struct {
	Engine domain.DatabaseEngine
	Now    func() time.Time
}

func (p Planner) PlanTimestamp(source domain.SourceID, target time.Time, base domain.SnapshotID, logs []domain.TransactionLog) (domain.PITRPlan, error) {
	eng := p.Engine
	if eng == "" {
		eng = domain.EngineMySQL
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	ids := make([]domain.TransactionLogID, 0, len(logs))
	for _, l := range logs {
		if l.Status == domain.LogVerified {
			ids = append(ids, l.ID)
		}
	}
	return domain.PITRPlan{ID: domain.PITRPlanID("mysqlpitr_" + now.Format("20060102150405")), SourceID: source, Engine: eng, Logs: ids, Target: domain.RecoveryTarget{Type: domain.RecoveryTargetTimestamp, Timestamp: &target}, CreatedAt: now, ExpiresAt: now.Add(time.Hour), Metadata: map[string]string{"base_snapshot": string(base)}}, nil
}
