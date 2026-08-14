package ports

import (
	"context"
	"io"

	"github.com/dbvault/dbvault/internal/domain"
)

type BackupPlanRequest struct {
	Source     domain.Source
	Inspection domain.DatabaseInspection
	Toolchain  domain.Toolchain
}

type BackupPlan struct {
	Engine        domain.DatabaseEngine
	Mode          domain.DatabaseBackupMode
	Format        domain.BackupFormat
	Artifacts     []domain.ArtifactSpec
	EstimatedSize int64
	Warnings      []string
}

type BackupRequest struct {
	Source    domain.Source
	Plan      BackupPlan
	Toolchain domain.Toolchain
}

type BackupVerificationRequest struct {
	Source    domain.Source
	BackupSet domain.BackupSet
	Artifacts ArtifactSource
}

type RestorePlanRequest struct {
	SourceSnapshot domain.Snapshot
	BackupSet      domain.BackupSet
	Target         map[string]any
}

type RestoreRequest struct {
	Plan   domain.RestorePlan
	Target map[string]any
}

type RestoredTargetVerificationRequest struct {
	Plan   domain.RestorePlan
	Target map[string]any
}

type DatabaseDriver interface {
	Descriptor() domain.DriverDescriptor
	ValidateSource(ctx context.Context, source domain.Source) error
	InspectSource(ctx context.Context, source domain.Source) (domain.DatabaseInspection, error)
	DetectToolchain(ctx context.Context, source domain.Source) (domain.Toolchain, error)
	PlanBackup(ctx context.Context, request BackupPlanRequest) (BackupPlan, error)
	CreateBackup(ctx context.Context, request BackupRequest, sink ArtifactSink) (domain.BackupSet, error)
	VerifyBackup(ctx context.Context, request BackupVerificationRequest) (domain.VerificationResult, error)
	PlanRestore(ctx context.Context, request RestorePlanRequest) (domain.RestorePlan, error)
	Restore(ctx context.Context, request RestoreRequest, source ArtifactSource) (domain.RestoreResult, error)
	VerifyRestoredTarget(ctx context.Context, request RestoredTargetVerificationRequest) (domain.VerificationResult, error)
}

type DatabaseDriverRegistry interface {
	Get(engine domain.DatabaseEngine) (DatabaseDriver, bool)
	List() []domain.DriverDescriptor
}

type ArtifactSink interface {
	OpenArtifact(ctx context.Context, spec domain.ArtifactSpec) (ArtifactWriter, error)
}

type ArtifactWriter interface {
	io.WriteCloser
	Commit(ctx context.Context, metadata domain.ArtifactCommitMetadata) (domain.StoredArtifact, error)
	Abort(ctx context.Context, cause error) error
}

type ArtifactSource interface {
	OpenArtifact(ctx context.Context, artifactID domain.ArtifactID) (io.ReadCloser, domain.BackupArtifact, error)
}
