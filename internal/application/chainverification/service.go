package chainverification

import (
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Service struct{ Now func() time.Time }

func (s Service) Validate(source domain.SourceID, engine domain.DatabaseEngine, bases []domain.PhysicalBackupSet, logs []domain.TransactionLog) domain.ChainValidationResult {
	at := time.Now()
	if s.Now != nil {
		at = s.Now()
	}
	res := domain.ChainValidationResult{SourceID: source, Engine: engine, Continuous: len(bases) > 0, CheckedAt: at}
	if len(bases) == 0 {
		res.Continuous = false
		res.Gaps = append(res.Gaps, domain.RecoveryGap{Reason: "no verified base backup"})
		return res
	}
	for _, l := range logs {
		if l.Status == domain.LogGap || l.Status == domain.LogCorrupted {
			res.Continuous = false
			res.Gaps = append(res.Gaps, domain.RecoveryGap{AfterPosition: l.StartPosition, BeforePosition: l.EndPosition, Reason: string(l.Status)})
		}
		if l.Status == domain.LogVerified {
			res.VerifiedLogs = append(res.VerifiedLogs, l.ID)
		}
	}
	return res
}
