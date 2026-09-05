package physical

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type stubRunner struct {
	lastReq ports.ProcessRequest
	runFn   func(ports.ProcessRequest) (ports.ProcessResult, error)
}

func (s *stubRunner) Run(ctx context.Context, req ports.ProcessRequest) (ports.ProcessResult, error) {
	s.lastReq = req
	if s.runFn != nil {
		return s.runFn(req)
	}
	return ports.ProcessResult{ExitCode: 0}, nil
}

func (s *stubRunner) Start(ctx context.Context, req ports.ProcessRequest) (ports.RunningProcess, error) {
	return nil, nil
}

type stubSink struct {
	spec   domain.ArtifactSpec
	buf    bytes.Buffer
	abort  bool
	commit bool
}

func (s *stubSink) OpenArtifact(ctx context.Context, spec domain.ArtifactSpec) (ports.ArtifactWriter, error) {
	s.spec = spec
	return &stubWriter{sink: s}, nil
}

type stubWriter struct {
	sink *stubSink
}

func (w *stubWriter) Write(p []byte) (int, error) {
	return w.sink.buf.Write(p)
}

func (w *stubWriter) Close() error {
	return nil
}

func (w *stubWriter) Commit(ctx context.Context, meta domain.ArtifactCommitMetadata) (domain.StoredArtifact, error) {
	w.sink.commit = true
	return domain.StoredArtifact{
		Artifact: domain.BackupArtifact{
			ID:          w.sink.spec.ID,
			Name:        w.sink.spec.Name,
			Format:      w.sink.spec.Format,
			ContentType: w.sink.spec.ContentType,
			LogicalSize: int64(w.sink.buf.Len()),
			RootDigest:  "digest_pg_base",
		},
	}, nil
}

func (w *stubWriter) Abort(ctx context.Context, cause error) error {
	w.sink.abort = true
	return nil
}

func TestPhysicalDescriptorAndCapabilities(t *testing.T) {
	d := New(nil, Config{})
	desc := d.Descriptor()
	if desc.Engine != domain.EnginePostgres || !desc.SupportsPhysicalBackup || !desc.SupportsIncremental {
		t.Fatalf("unexpected descriptor: %+v", desc)
	}

	caps, err := d.InspectRecoveryCapabilities(context.Background(), domain.Source{})
	if err != nil {
		t.Fatal(err)
	}
	if !caps.PhysicalBackup || !caps.ContinuousLogArchiving || !caps.TimestampRecovery {
		t.Fatalf("unexpected recovery capabilities: %+v", caps)
	}
}

func TestPlanBaseBackup(t *testing.T) {
	d := New(nil, Config{Format: "tar", BackupType: domain.PhysicalBackupFull})
	plan, err := d.PlanBaseBackup(context.Background(), ports.BaseBackupPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Engine != domain.EnginePostgres || plan.BackupType != domain.PhysicalBackupFull {
		t.Fatalf("unexpected base backup plan: %+v", plan)
	}
}

func TestCreateBaseBackup(t *testing.T) {
	var capturedReq ports.ProcessRequest
	runner := &stubRunner{
		runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
			capturedReq = req
			_, _ = req.StandardOutput.Write([]byte("tar-stream-content"))
			return ports.ProcessResult{ExitCode: 0}, nil
		},
	}

	cfg := Config{
		Format:          "tar",
		Checkpoint:      "fast",
		IncludeWAL:      true,
		VerifyChecksums: true,
		MaximumRate:     "100M",
		Progress:        true,
		Host:            "127.0.0.1",
		Port:            5432,
		Username:        "postgres",
		Password:        "pg_secret",
	}

	d := New(runner, cfg)
	sink := &stubSink{}
	set, err := d.CreateBaseBackup(context.Background(), ports.BaseBackupRequest{
		Source: domain.Source{ID: "src-pg"},
		Plan:   ports.BaseBackupPlan{BackupType: domain.PhysicalBackupFull},
		Target: domain.Repository{ID: "repo-1"},
	}, sink)

	if err != nil {
		t.Fatal(err)
	}

	if set.Engine != domain.EnginePostgres || set.SourceID != "src-pg" {
		t.Fatalf("unexpected backup set: %+v", set)
	}
	if !sink.commit || sink.abort {
		t.Fatalf("expected commit=true abort=false, got commit=%v abort=%v", sink.commit, sink.abort)
	}

	if capturedReq.Executable != "pg_basebackup" {
		t.Fatalf("expected pg_basebackup, got %s", capturedReq.Executable)
	}
	argsStr := strings.Join(capturedReq.Arguments, " ")
	for _, expected := range []string{
		"--format=tar",
		"--pgdata=-",
		"--checkpoint=fast",
		"--wal-method=stream",
		"--max-rate=100M",
		"--progress",
		"-h 127.0.0.1",
		"-p 5432",
		"-U postgres",
		"--no-password",
		"--label=dbvault_",
	} {
		if !strings.Contains(argsStr, expected) {
			t.Fatalf("args %q missing %q", argsStr, expected)
		}
	}

	// Verify environment credentials
	hasPwd := false
	for _, env := range capturedReq.Environment {
		if env.Name == "PGPASSWORD" && env.Value == "pg_secret" && env.Sensitive {
			hasPwd = true
			break
		}
	}
	if !hasPwd {
		t.Fatalf("expected PGPASSWORD in environment, got %+v", capturedReq.Environment)
	}
}

func TestConfigureAndValidateRecoveryChain(t *testing.T) {
	d := New(nil, Config{})

	cfg, err := d.ConfigureLogCollection(context.Background(), ports.LogCollectionConfigurationRequest{
		Source: domain.Source{ID: "src-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "replication" || cfg.Engine != domain.EnginePostgres {
		t.Fatalf("unexpected log collection config: %+v", cfg)
	}

	chainRes, err := d.ValidateRecoveryChain(context.Background(), ports.ChainValidationRequest{
		SourceID:    "src-1",
		BaseBackups: []domain.PhysicalBackupSet{{ID: "base-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !chainRes.Continuous || chainRes.Engine != domain.EnginePostgres {
		t.Fatalf("unexpected chain result: %+v", chainRes)
	}
}

func TestPlanAndExecutePITR(t *testing.T) {
	d := New(nil, Config{})
	now := time.Now()

	plan, err := d.PlanPointInTimeRecovery(context.Background(), ports.PITRPlanRequest{
		SourceID:    "src-1",
		Target:      domain.RecoveryTarget{Type: domain.RecoveryTargetTimestamp, Timestamp: &now},
		BaseBackups: []domain.PhysicalBackupSet{{ID: "base-1"}},
		Logs:        []domain.TransactionLog{{ID: "wal-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Engine != domain.EnginePostgres || len(plan.BaseBackups) != 1 || len(plan.Logs) != 1 {
		t.Fatalf("unexpected PITR plan: %+v", plan)
	}

	res, err := d.ExecutePointInTimeRecovery(context.Background(), ports.PITRExecutionRequest{
		Plan: plan,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Succeeded || !res.ReachedTarget {
		t.Fatalf("unexpected PITR result: %+v", res)
	}
}
