package recovery

import (
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Planner struct{ Now func() time.Time }

func (p Planner) PlanTimestamp(source domain.SourceID, target time.Time, backups []domain.PhysicalBackupSet, logs []domain.TransactionLog) (domain.PITRPlan, error) {
	if len(backups) == 0 {
		return domain.PITRPlan{}, domain.NewError(domain.ErrRecoveryWindowUnavailable, "no PostgreSQL physical base backup", nil)
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
	return domain.PITRPlan{ID: domain.PITRPlanID("pgpitr_" + now.Format("20060102150405")), SourceID: source, Engine: domain.EnginePostgres, BaseBackups: []domain.PhysicalBackupID{backups[0].ID}, Logs: ids, Target: domain.RecoveryTarget{Type: domain.RecoveryTargetTimestamp, Timestamp: &target, Action: "promote"}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}
