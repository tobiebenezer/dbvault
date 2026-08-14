package ports

import (
	"context"
	"github.com/dbvault/dbvault/internal/domain"
)

type ScratchReservation struct {
	Dir     string
	Release func() error
}

type ScratchManager interface {
	Reserve(ctx context.Context, runID domain.BackupRunID, estimatedBytes int64) (ScratchReservation, error)
	CleanupAbandoned(ctx context.Context) error
}
