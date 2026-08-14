package scratch

import (
	"context"
	"fmt"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
	"os"
	"path/filepath"
)

type Manager struct {
	Root             string
	MinimumFreeBytes int64
}

func New(root string) *Manager { return &Manager{Root: root} }
func (m *Manager) Reserve(ctx context.Context, runID domain.BackupRunID, estimatedBytes int64) (ports.ScratchReservation, error) {
	if err := os.MkdirAll(m.Root, 0700); err != nil {
		return ports.ScratchReservation{}, err
	}
	dir := filepath.Join(m.Root, fmt.Sprintf("run-%s", runID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ports.ScratchReservation{}, err
	}
	return ports.ScratchReservation{Dir: dir, Release: func() error { return os.RemoveAll(dir) }}, nil
}
func (m *Manager) CleanupAbandoned(ctx context.Context) error { return os.MkdirAll(m.Root, 0700) }
