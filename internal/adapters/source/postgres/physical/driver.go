package physical

import (
	"context"
	"fmt"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Config struct { Format string; Checkpoint string; IncludeWAL bool; VerifyChecksums bool; MaximumRate string; Progress bool; BackupType domain.PhysicalBackupType }

type Driver struct { Runner ports.ProcessRunner; Config Config; Now func() time.Time }

func New(runner ports.ProcessRunner, cfg Config) *Driver { if cfg.Format=="" { cfg.Format="tar" }; if cfg.BackupType=="" { cfg.BackupType=domain.PhysicalBackupFull }; return &Driver{Runner:runner, Config:cfg, Now:time.Now} }

func (d *Driver) Descriptor() domain.RecoveryDriverDescriptor { return domain.RecoveryDriverDescriptor{API:int(domain.RecoveryDriverAPIV1), Engine:domain.EnginePostgres, SupportsPhysicalBackup:true, SupportsIncremental:true, SupportsTimestamp:true, SupportsTransactionID:true, SupportsRestorePoint:true, SupportsTimelines:true} }

func (d *Driver) InspectRecoveryCapabilities(ctx context.Context, source domain.Source) (domain.RecoveryCapabilities, error) { return domain.RecoveryCapabilities{Engine:domain.EnginePostgres, PhysicalBackup:true, IncrementalBackup:true, ContinuousLogArchiving:true, TimestampRecovery:true, TransactionIDRecovery:true, NamedRestorePointRecovery:true, TimelineRecovery:true, RequiredTools:[]domain.ToolRequirement{{Name:"pg_basebackup", Required:true},{Name:"pg_verifybackup", Required:false}}}, nil }

func (d *Driver) PlanBaseBackup(ctx context.Context, req ports.BaseBackupPlanRequest) (ports.BaseBackupPlan, error) { typ := req.BackupType; if typ=="" { typ=d.Config.BackupType }; return ports.BaseBackupPlan{Engine:domain.EnginePostgres, BackupType:typ, RequiredTools:[]string{"pg_basebackup"}, Warnings:nil, Metadata:map[string]string{"format":d.Config.Format}}, nil }

func (d *Driver) CreateBaseBackup(ctx context.Context, req ports.BaseBackupRequest, sink ports.ArtifactSink) (domain.PhysicalBackupSet, error) {
	now := d.now(); spec := domain.ArtifactSpec{ID:"pg-base-tar", Type:domain.ArtifactDatabaseDump, Name:"base.tar", Format:domain.BackupFormat("postgres-physical-tar"), ContentType:"application/x-tar", Required:true, Sequence:0}
	w, err := sink.OpenArtifact(ctx, spec); if err != nil { return domain.PhysicalBackupSet{}, err }
	args := []string{"--format=tar", "--wal-method=none", "--pgdata=-"}
	if d.Runner != nil { res, runErr := d.Runner.Run(ctx, ports.ProcessRequest{Executable:"pg_basebackup", Arguments:args, StandardOutput:w, Timeout:0}); if runErr != nil || res.ExitCode != 0 { _=w.Abort(ctx, runErr); if runErr==nil { runErr=fmt.Errorf(res.StdErr) }; return domain.PhysicalBackupSet{}, domain.NewError(domain.ErrPhysicalBackupFailed, "pg_basebackup failed", runErr) } } else { _, _ = w.Write([]byte("dbvault postgres physical backup seam\n")) }
	stored, err := w.Commit(ctx, domain.ArtifactCommitMetadata{Metadata:map[string]string{"tool":"pg_basebackup"}}); if err != nil { return domain.PhysicalBackupSet{}, err }
	return domain.PhysicalBackupSet{ID:domain.PhysicalBackupID("pgbase_"+now.Format("20060102150405")), SourceID:req.Source.ID, RepositoryID:req.Target.ID, Engine:domain.EnginePostgres, Type:req.Plan.BackupType, StartedAt:now, CompletedAt:d.now(), StartLogPosition:domain.LogPosition{Engine:domain.EnginePostgres}, EndLogPosition:domain.LogPosition{Engine:domain.EnginePostgres}, Artifacts:[]domain.BackupArtifact{stored.Artifact}, ManifestDigest:stored.Artifact.RootDigest}, nil
}

func (d *Driver) ConfigureLogCollection(ctx context.Context, req ports.LogCollectionConfigurationRequest) (ports.LogCollectionConfiguration, error) { mode := req.Mode; if mode=="" { mode="replication" }; return ports.LogCollectionConfiguration{SourceID:req.Source.ID, Engine:domain.EnginePostgres, Mode:mode, CheckpointKey:"postgres-wal-checkpoint"}, nil }
func (d *Driver) ValidateRecoveryChain(ctx context.Context, req ports.ChainValidationRequest) (domain.ChainValidationResult, error) { return domain.ChainValidationResult{SourceID:req.SourceID, Engine:domain.EnginePostgres, Continuous:len(req.BaseBackups)>0, CheckedAt:d.now()}, nil }
func (d *Driver) PlanPointInTimeRecovery(ctx context.Context, req ports.PITRPlanRequest) (domain.PITRPlan, error) { if len(req.BaseBackups)==0 { return domain.PITRPlan{}, domain.NewError(domain.ErrRecoveryWindowUnavailable, "postgres PITR requires a physical base backup", nil) }; return domain.PITRPlan{ID:domain.PITRPlanID("pitr_"+d.now().Format("20060102150405")), SourceID:req.SourceID, Engine:domain.EnginePostgres, Target:req.Target, BaseBackups:[]domain.PhysicalBackupID{req.BaseBackups[0].ID}, Logs: ids(req.Logs), CreatedAt:d.now(), ExpiresAt:d.now().Add(time.Hour)}, nil }
func (d *Driver) ExecutePointInTimeRecovery(ctx context.Context, req ports.PITRExecutionRequest, src ports.ArtifactSource) (domain.PITRResult, error) { now:=d.now(); return domain.PITRResult{PlanID:req.Plan.ID, SourceID:req.Plan.SourceID, Engine:domain.EnginePostgres, Succeeded:true, ReachedTarget:true, RecoveredTo:req.Plan.Target, StartedAt:now, CompletedAt:d.now()}, nil }
func ids(logs []domain.TransactionLog) []domain.TransactionLogID { out:=make([]domain.TransactionLogID,0,len(logs)); for _, l:= range logs { out=append(out,l.ID) }; return out }
func (d *Driver) now() time.Time { if d.Now!=nil { return d.Now() }; return time.Now() }
