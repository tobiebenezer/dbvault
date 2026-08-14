package ports

import (
	"context"

	"github.com/dbvault/dbvault/internal/domain"
)

type BaseBackupPlanRequest struct { Source domain.Source; Repository domain.Repository; BackupType domain.PhysicalBackupType }
type BaseBackupPlan struct { Engine domain.DatabaseEngine; BackupType domain.PhysicalBackupType; EstimatedSize int64; RequiredTools []string; Warnings []string; Metadata map[string]string }
type BaseBackupRequest struct { Source domain.Source; Plan BaseBackupPlan; Target domain.Repository }

type LogCollectionConfigurationRequest struct { Source domain.Source; Mode string; SlotName string }
type LogCollectionConfiguration struct { SourceID domain.SourceID; Engine domain.DatabaseEngine; Mode string; CheckpointKey string; Warnings []string }

type ChainValidationRequest struct { SourceID domain.SourceID; BaseBackups []domain.PhysicalBackupSet; Logs []domain.TransactionLog; Timelines []domain.TimelineHistory }
type PITRPlanRequest struct { SourceID domain.SourceID; Engine domain.DatabaseEngine; Target domain.RecoveryTarget; BaseBackups []domain.PhysicalBackupSet; Logs []domain.TransactionLog; Windows []domain.RecoveryWindow }
type PITRExecutionRequest struct { Plan domain.PITRPlan; TargetDirectory string; ReplaceExisting bool }

type RecoveryDriver interface {
	Descriptor() domain.RecoveryDriverDescriptor
	InspectRecoveryCapabilities(ctx context.Context, source domain.Source) (domain.RecoveryCapabilities, error)
	PlanBaseBackup(ctx context.Context, request BaseBackupPlanRequest) (BaseBackupPlan, error)
	CreateBaseBackup(ctx context.Context, request BaseBackupRequest, sink ArtifactSink) (domain.PhysicalBackupSet, error)
	ConfigureLogCollection(ctx context.Context, request LogCollectionConfigurationRequest) (LogCollectionConfiguration, error)
	ValidateRecoveryChain(ctx context.Context, request ChainValidationRequest) (domain.ChainValidationResult, error)
	PlanPointInTimeRecovery(ctx context.Context, request PITRPlanRequest) (domain.PITRPlan, error)
	ExecutePointInTimeRecovery(ctx context.Context, request PITRExecutionRequest, source ArtifactSource) (domain.PITRResult, error)
}

type RecoveryDriverRegistry interface { Get(engine domain.DatabaseEngine) (RecoveryDriver, bool); List() []domain.RecoveryDriverDescriptor }
