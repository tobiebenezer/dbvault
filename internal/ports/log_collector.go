package ports

import (
	"context"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type CollectorStatus struct {
	SourceID     domain.SourceID
	Engine       domain.DatabaseEngine
	Running      bool
	LastPosition domain.LogPosition
	LastError    string
	UpdatedAt    time.Time
}
type CollectorCheckpoint struct {
	SourceID  domain.SourceID
	Engine    domain.DatabaseEngine
	LineageID domain.LineageID
	Position  domain.LogPosition
	UpdatedAt time.Time
}

type LogCollector interface {
	Start(context.Context) error
	Stop(context.Context) error
	Status(context.Context) (CollectorStatus, error)
	Checkpoint(context.Context) (CollectorCheckpoint, error)
}

type LogRepository interface {
	PutLog(context.Context, domain.TransactionLog, ArtifactSource) error
	GetLog(context.Context, domain.TransactionLogID) (domain.TransactionLog, error)
	ListLogs(context.Context, domain.SourceID) ([]domain.TransactionLog, error)
}
