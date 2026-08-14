package recoverywindow

import (
	"sort"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Calculator struct{ Now func() time.Time }

func (c Calculator) Calculate(source domain.SourceID, lineage domain.LineageID, bases []domain.PhysicalBackupSet, logs []domain.TransactionLog) []domain.RecoveryWindow {
	if len(bases) == 0 {
		return nil
	}
	sort.Slice(bases, func(i, j int) bool { return bases[i].StartedAt.Before(bases[j].StartedAt) })
	sort.Slice(logs, func(i, j int) bool { return logs[i].CollectedAt.Before(logs[j].CollectedAt) })
	at := time.Now()
	if c.Now != nil {
		at = c.Now()
	}
	windows := []domain.RecoveryWindow{}
	base := bases[0]
	current := domain.RecoveryWindow{SourceID: source, LineageID: lineage, BaseBackupID: string(base.ID), EarliestTime: base.CompletedAt, LatestTime: base.CompletedAt, Continuous: true, LastVerifiedAt: at, DestinationCoverage: map[domain.DestinationID]domain.DestinationRecoveryCoverage{}}
	var last *domain.TransactionLog
	for _, lg := range logs {
		if lg.Status == domain.LogGap || lg.Status == domain.LogCorrupted {
			if !current.EarliestTime.IsZero() {
				current.GapCount++
				windows = append(windows, current)
			}
			current = domain.RecoveryWindow{SourceID: source, LineageID: lineage, BaseBackupID: string(base.ID), Continuous: false, GapCount: 1, LastVerifiedAt: at}
			last = nil
			continue
		}
		if lg.StartTime != nil && current.EarliestTime.IsZero() {
			current.EarliestTime = *lg.StartTime
		}
		if lg.EndTime != nil {
			current.LatestTime = *lg.EndTime
		}
		current.LatestPosition = lg.EndPosition
		current.Continuous = true
		copy := lg
		last = &copy
	}
	if last != nil || len(windows) == 0 {
		windows = append(windows, current)
	}
	return windows
}
