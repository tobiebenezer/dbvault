package postgres

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	return nil, errors.New("not implemented")
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
			RootDigest:  "dummy-root-digest",
		},
	}, nil
}

func (w *stubWriter) Abort(ctx context.Context, cause error) error {
	w.sink.abort = true
	return nil
}

type stubSource struct {
	content []byte
	format  domain.BackupFormat
	name    string
}

func (s *stubSource) OpenArtifact(ctx context.Context, id domain.ArtifactID) (io.ReadCloser, domain.BackupArtifact, error) {
	return io.NopCloser(bytes.NewReader(s.content)), domain.BackupArtifact{ID: id, Name: s.name, Format: s.format}, nil
}

func baseConfig() Config {
	return Config{
		Host:           "127.0.0.1",
		Port:           5432,
		Database:       "app_db",
		Username:       "dbvault",
		SSLMode:        "prefer",
		ConnectTimeout: 10 * time.Second,
	}
}

func TestDescriptor(t *testing.T) {
	d := New(&stubRunner{}, baseConfig())
	desc := d.Descriptor()
	if desc.API != domain.DatabaseDriverAPIV1 {
		t.Fatalf("unexpected API: %v", desc.API)
	}
	if desc.Engine != domain.EnginePostgres {
		t.Fatalf("unexpected engine: %v", desc.Engine)
	}
	if !desc.SupportsStreaming || !desc.SupportsParallel || !desc.SupportsGlobals {
		t.Fatalf("unexpected capabilities: %+v", desc)
	}
}

func TestValidateSource(t *testing.T) {
	t.Run("valid default config", func(t *testing.T) {
		d := New(&stubRunner{}, baseConfig())
		if err := d.ValidateSource(context.Background(), domain.Source{}); err != nil {
			t.Fatalf("expected valid: %v", err)
		}
	})

	t.Run("missing runner", func(t *testing.T) {
		d := New(nil, baseConfig())
		if err := d.ValidateSource(context.Background(), domain.Source{}); err == nil {
			t.Fatal("expected error for nil runner")
		}
	})

	t.Run("missing host", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Host = ""
		d := New(&stubRunner{}, cfg)
		if err := d.ValidateSource(context.Background(), domain.Source{}); err == nil {
			t.Fatal("expected error for missing host")
		}
	})

	t.Run("missing database", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Database = ""
		d := New(&stubRunner{}, cfg)
		if err := d.ValidateSource(context.Background(), domain.Source{}); err == nil {
			t.Fatal("expected error for missing database")
		}
	})

	t.Run("missing username", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Username = ""
		d := New(&stubRunner{}, cfg)
		if err := d.ValidateSource(context.Background(), domain.Source{}); err == nil {
			t.Fatal("expected error for missing username")
		}
	})

	t.Run("password resolution from file", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "pg_pass")
		if err := os.WriteFile(tmp, []byte("pg_secret_123\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg := baseConfig()
		cfg.Password = SecretReference{File: tmp}
		d := New(&stubRunner{}, cfg)
		pwd, err := d.password()
		if err != nil || pwd != "pg_secret_123" {
			t.Fatalf("expected pg_secret_123, got %q (err: %v)", pwd, err)
		}
		if err := d.ValidateSource(context.Background(), domain.Source{}); err != nil {
			t.Fatalf("unexpected validation failure: %v", err)
		}
	})

	t.Run("password resolution from env", func(t *testing.T) {
		t.Setenv("PG_TEST_SECRET", "pg_env_pwd")
		cfg := baseConfig()
		cfg.Password = SecretReference{Env: "PG_TEST_SECRET"}
		d := New(&stubRunner{}, cfg)
		pwd, err := d.password()
		if err != nil || pwd != "pg_env_pwd" {
			t.Fatalf("expected pg_env_pwd, got %q (err: %v)", pwd, err)
		}
	})

	t.Run("invalid password file", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Password = SecretReference{File: "/nonexistent/path/to/pwd"}
		d := New(&stubRunner{}, cfg)
		if err := d.ValidateSource(context.Background(), domain.Source{}); err == nil {
			t.Fatal("expected error for non-existent password file")
		}
	})
}

func TestPlanBackup(t *testing.T) {
	t.Run("custom format", func(t *testing.T) {
		cfg := baseConfig()
		cfg.BackupFormat = "postgres-custom"
		d := New(&stubRunner{}, cfg)
		plan, err := d.PlanBackup(context.Background(), ports.BackupPlanRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Format != domain.FormatPostgresCustom || len(plan.Artifacts) != 1 {
			t.Fatalf("unexpected plan: %+v", plan)
		}
		if plan.Artifacts[0].Name != "database.dump" {
			t.Fatalf("expected database.dump, got %s", plan.Artifacts[0].Name)
		}
	})

	t.Run("plain sql format", func(t *testing.T) {
		cfg := baseConfig()
		cfg.BackupFormat = "postgres-plain-sql"
		d := New(&stubRunner{}, cfg)
		plan, err := d.PlanBackup(context.Background(), ports.BackupPlanRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Format != domain.FormatPostgresPlainSQL || len(plan.Artifacts) != 1 {
			t.Fatalf("unexpected plan: %+v", plan)
		}
		if plan.Artifacts[0].Name != "database.sql" {
			t.Fatalf("expected database.sql, got %s", plan.Artifacts[0].Name)
		}
	})

	t.Run("directory format with globals", func(t *testing.T) {
		cfg := baseConfig()
		cfg.BackupFormat = "postgres-directory"
		cfg.IncludeGlobals = true
		d := New(&stubRunner{}, cfg)
		plan, err := d.PlanBackup(context.Background(), ports.BackupPlanRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Format != domain.FormatPostgresDirectory || len(plan.Artifacts) != 2 {
			t.Fatalf("unexpected plan: %+v", plan)
		}
		if plan.Artifacts[0].Type != domain.ArtifactGlobals {
			t.Fatalf("expected first artifact to be globals, got %+v", plan.Artifacts[0])
		}
	})

	t.Run("unsupported format", func(t *testing.T) {
		cfg := baseConfig()
		cfg.BackupFormat = "invalid-fmt"
		d := New(&stubRunner{}, cfg)
		if _, err := d.PlanBackup(context.Background(), ports.BackupPlanRequest{}); err == nil {
			t.Fatal("expected error for unsupported format")
		}
	})
}

func TestCommandFor(t *testing.T) {
	cfg := baseConfig()
	cfg.IncludeOwnership = false
	cfg.IncludePrivileges = false
	cfg.ParallelJobs = 4
	cfg.SchemaInclude = []string{"public", "analytics"}
	cfg.SchemaExclude = []string{"staging"}
	cfg.TableInclude = []string{"users", "orders"}
	cfg.TableExclude = []string{"temp_log"}
	cfg.ExtraOptions = []string{"--blobs"}

	d := New(&stubRunner{}, cfg)

	t.Run("custom dump args", func(t *testing.T) {
		spec := domain.ArtifactSpec{
			ID:     "main",
			Format: domain.FormatPostgresCustom,
		}
		cmd, err := d.commandFor(spec)
		if err != nil {
			t.Fatal(err)
		}
		if cmd[0] != "pg_dump" {
			t.Fatalf("expected pg_dump executable, got %s", cmd[0])
		}
		cmdStr := strings.Join(cmd, " ")
		for _, expected := range []string{
			"--format=custom",
			"--file=-",
			"--no-owner",
			"--no-privileges",
			"--schema public",
			"--schema analytics",
			"--exclude-schema staging",
			"--table users",
			"--table orders",
			"--exclude-table temp_log",
			"--blobs",
			"host=127.0.0.1",
			"port=5432",
			"dbname=app_db",
			"user=dbvault",
		} {
			if !strings.Contains(cmdStr, expected) {
				t.Fatalf("command string %q missing expected part: %q", cmdStr, expected)
			}
		}
	})

	t.Run("directory format args", func(t *testing.T) {
		spec := domain.ArtifactSpec{
			ID:     "main",
			Format: domain.FormatPostgresDirectory,
		}
		cmd, err := d.commandFor(spec)
		if err == nil {
			t.Fatalf("expected directory format to be rejected, got command %v", cmd)
		}
		if !strings.Contains(err.Error(), "directory format requires scratch artifact support") {
			t.Fatalf("expected clear directory-format error, got: %v", err)
		}
	})

	t.Run("globals args", func(t *testing.T) {
		spec := domain.ArtifactSpec{
			ID:   "globals",
			Type: domain.ArtifactGlobals,
		}
		cmd, err := d.commandFor(spec)
		if err != nil {
			t.Fatal(err)
		}
		if cmd[0] != "pg_dumpall" {
			t.Fatalf("expected pg_dumpall, got %s", cmd[0])
		}
		cmdStr := strings.Join(cmd, " ")
		for _, expected := range []string{"--globals-only", "-h 127.0.0.1", "-p 5432", "-U dbvault", "-d app_db"} {
			if !strings.Contains(cmdStr, expected) {
				t.Fatalf("globals command %q missing %q", cmdStr, expected)
			}
		}
	})
}

func TestCreateBackup(t *testing.T) {
	runner := &stubRunner{
		runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
			_, _ = req.StandardOutput.Write([]byte("mock-pg-dump-bytes"))
			return ports.ProcessResult{ExitCode: 0}, nil
		},
	}
	d := New(runner, baseConfig())
	sink := &stubSink{}
	req := ports.BackupRequest{
		Plan: ports.BackupPlan{
			Format: domain.FormatPostgresCustom,
			Artifacts: []domain.ArtifactSpec{
				{
					ID:          "postgres-main",
					Type:        domain.ArtifactDatabaseDump,
					Name:        "database.dump",
					Format:      domain.FormatPostgresCustom,
					ContentType: "application/octet-stream",
				},
			},
		},
	}

	set, err := d.CreateBackup(context.Background(), req, sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Artifacts) != 1 || set.Engine != domain.EnginePostgres {
		t.Fatalf("unexpected backup set: %+v", set)
	}
	if !sink.commit || sink.abort {
		t.Fatalf("expected commit=true abort=false, got commit=%v abort=%v", sink.commit, sink.abort)
	}
	if sink.buf.String() != "mock-pg-dump-bytes" {
		t.Fatalf("unexpected sink content: %q", sink.buf.String())
	}
}

func TestCreateBackupFailure(t *testing.T) {
	runner := &stubRunner{
		runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
			return ports.ProcessResult{ExitCode: 1, StdErr: "FATAL: database app_db does not exist"}, nil
		},
	}
	d := New(runner, baseConfig())
	sink := &stubSink{}
	req := ports.BackupRequest{
		Plan: ports.BackupPlan{
			Format: domain.FormatPostgresCustom,
			Artifacts: []domain.ArtifactSpec{
				{ID: "postgres-main", Format: domain.FormatPostgresCustom},
			},
		},
	}

	_, err := d.CreateBackup(context.Background(), req, sink)
	if err == nil {
		t.Fatal("expected backup creation failure")
	}
	if !sink.abort {
		t.Fatal("expected sink to be aborted on error")
	}
}

func TestRestore(t *testing.T) {
	t.Run("restore custom format using pg_restore", func(t *testing.T) {
		var capturedReq ports.ProcessRequest
		runner := &stubRunner{
			runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
				capturedReq = req
				return ports.ProcessResult{ExitCode: 0}, nil
			},
		}
		cfg := baseConfig()
		cfg.ParallelJobs = 2
		d := New(runner, cfg)
		src := &stubSource{
			content: []byte("dump-payload"),
			format:  domain.FormatPostgresCustom,
			name:    "database.dump",
		}
		req := ports.RestoreRequest{
			Plan: domain.RestorePlan{
				SnapshotID: "snap1",
				Artifacts:  []domain.ArtifactID{"art1"},
			},
		}
		res, err := d.Restore(context.Background(), req, src)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Succeeded || res.Engine != domain.EnginePostgres {
			t.Fatalf("unexpected restore result: %+v", res)
		}
		if capturedReq.Executable != "pg_restore" {
			t.Fatalf("expected pg_restore, got %s", capturedReq.Executable)
		}
		argsStr := strings.Join(capturedReq.Arguments, " ")
		for _, exp := range []string{"--clean", "--if-exists", "--no-owner", "--no-privileges", "-h 127.0.0.1", "-p 5432", "-U dbvault", "-d app_db", "--jobs 2"} {
			if !strings.Contains(argsStr, exp) {
				t.Fatalf("args %q missing %q", argsStr, exp)
			}
		}
	})

	t.Run("restore plain sql format using psql", func(t *testing.T) {
		var capturedReq ports.ProcessRequest
		runner := &stubRunner{
			runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
				capturedReq = req
				return ports.ProcessResult{ExitCode: 0}, nil
			},
		}
		d := New(runner, baseConfig())
		src := &stubSource{
			content: []byte("CREATE TABLE foo();"),
			format:  domain.FormatPostgresPlainSQL,
			name:    "database.sql",
		}
		req := ports.RestoreRequest{
			Plan: domain.RestorePlan{
				SnapshotID: "snap2",
				Artifacts:  []domain.ArtifactID{"art2"},
			},
		}
		res, err := d.Restore(context.Background(), req, src)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Succeeded {
			t.Fatalf("restore failed: %+v", res)
		}
		if capturedReq.Executable != "psql" {
			t.Fatalf("expected psql, got %s", capturedReq.Executable)
		}
	})

	t.Run("restore failure classification", func(t *testing.T) {
		runner := &stubRunner{
			runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
				return ports.ProcessResult{ExitCode: 1, StdErr: "error connecting to server"}, nil
			},
		}
		d := New(runner, baseConfig())
		src := &stubSource{
			content: []byte("dummy"),
			format:  domain.FormatPostgresCustom,
			name:    "database.dump",
		}
		req := ports.RestoreRequest{
			Plan: domain.RestorePlan{
				Artifacts: []domain.ArtifactID{"art1"},
			},
		}
		_, err := d.Restore(context.Background(), req, src)
		if err == nil {
			t.Fatal("expected restore error")
		}
		appErr, ok := err.(*domain.AppError)
		if !ok || appErr.Code != domain.ErrRestoreToolFailed {
			t.Fatalf("expected ErrRestoreToolFailed, got %v", err)
		}
	})
}

func TestSnapshotSource(t *testing.T) {
	runner := &stubRunner{
		runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
			_, _ = req.StandardOutput.Write([]byte("mock-pg-dump-stream"))
			return ports.ProcessResult{ExitCode: 0}, nil
		},
	}
	d := New(runner, baseConfig())
	snapSrc := NewSnapshotSource(d)

	if snapSrc.Name() != "postgres" {
		t.Fatalf("expected name postgres, got %s", snapSrc.Name())
	}
	if err := snapSrc.Validate(context.Background(), domain.Source{}); err != nil {
		t.Fatal(err)
	}

	scratch := t.TempDir()
	art, err := snapSrc.CreateSnapshot(context.Background(), ports.SnapshotRequest{
		RunID:            "run-1",
		Source:           domain.Source{ID: "pg-src"},
		ScratchDirectory: scratch,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer art.Close()

	if art.Size() != int64(len("mock-pg-dump-stream")) {
		t.Fatalf("expected size %d, got %d", len("mock-pg-dump-stream"), art.Size())
	}
	if art.Metadata().PageSize != 1 || art.Metadata().SchemaDigest == "" {
		t.Fatalf("unexpected metadata: %+v", art.Metadata())
	}
}
