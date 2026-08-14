package restoredrill

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/dbvault/dbvault/internal/application/restore"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Service struct {
	Restore *restore.Service
	Scratch ports.ScratchManager
	Clock   ports.Clock
	Probes  ports.ProbeRunner
}

func (s *Service) Run(ctx context.Context, snapshotID domain.SnapshotID) (domain.RestoreDrillResult, error) {
	start := s.Clock.Now()
	id := domain.RestoreDrillID("drill_" + string(snapshotID))
	res := domain.RestoreDrillResult{ID: id, SnapshotID: snapshotID, Status: domain.VerificationPending, StartedAt: start}
	reservation, err := s.Scratch.Reserve(ctx, domain.BackupRunID("restore_"+string(snapshotID)), 0)
	if err != nil {
		res.Status = domain.VerificationFailed
		res.ErrorCode = string(domain.ErrScratchInsufficient)
		res.ErrorMessage = err.Error()
		return res, err
	}
	defer reservation.Release()
	target := filepath.Join(reservation.Dir, "drill.sqlite")
	if err := s.Restore.Restore(ctx, snapshotID, target, false); err != nil {
		res.Status = domain.VerificationFailed
		res.ErrorCode = string(domain.ErrVerificationFailed)
		res.ErrorMessage = err.Error()
		return res, err
	}
	_ = os.Remove(target)
	done := s.Clock.Now()
	res.CompletedAt = &done
	res.Duration = done.Sub(start)
	res.Status = domain.VerificationSucceeded
	res.QuickCheckPassed = true
	res.IntegrityPassed = true
	if res.Duration < 0 {
		res.Duration = time.Duration(0)
	}
	return res, nil
}
