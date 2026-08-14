package ports

import (
	"context"
	"github.com/dbvault/dbvault/internal/domain"
)

type ProbeResult struct {
	ID      string
	Passed  bool
	Message string
}

type ProbeRunner interface {
	Run(ctx context.Context, sqlitePath string, probes []domain.Probe) ([]ProbeResult, error)
}
