package mysql

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func baseConfig() Config {
	return Config{
		Engine:   "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "app",
		Username: "backup",
	}
}

func findEnv(vars []ports.EnvironmentVariable, name string) (ports.EnvironmentVariable, bool) {
	for _, v := range vars {
		if v.Name == name {
			return v, true
		}
	}
	return ports.EnvironmentVariable{}, false
}

func TestPasswordResolvesFromFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mysql-pwd")
	if err := os.WriteFile(file, []byte("s3cret-from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := baseConfig()
	cfg.Password = SecretReference{File: file}
	d := New(&stubRunner{}, cfg)
	got, err := d.password()
	if err != nil {
		t.Fatal(err)
	}
	if got != "s3cret-from-file" {
		t.Fatalf("password=%q", got)
	}
	envVars := d.env()
	v, ok := findEnv(envVars, "MYSQL_PWD")
	if !ok || v.Value != "s3cret-from-file" || !v.Sensitive {
		t.Fatalf("env=%+v ok=%v", envVars, ok)
	}
	// Verify redactions
	reds := d.redactions()
	if len(reds) == 0 || reds[0] != "s3cret-from-file" {
		t.Fatalf("expected redactions to contain password, got %+v", reds)
	}
}

func TestPasswordResolvesFromNamedEnvVar(t *testing.T) {
	t.Setenv("DBVAULT_TEST_MYSQL_PWD", "s3cret-from-env")
	cfg := baseConfig()
	cfg.Password = SecretReference{Env: "DBVAULT_TEST_MYSQL_PWD"}
	d := New(&stubRunner{}, cfg)
	envVars := d.env()
	v, ok := findEnv(envVars, "MYSQL_PWD")
	if !ok || v.Value != "s3cret-from-env" {
		t.Fatalf("env=%+v ok=%v", envVars, ok)
	}
}

func TestValidateSourceRequiresPasswordOnTCP(t *testing.T) {
	cfg := baseConfig()
	cfg.Password = SecretReference{}
	d := New(&stubRunner{}, cfg)
	err := d.ValidateSource(context.Background(), domain.Source{})
	if err == nil {
		t.Fatal("expected loud validation failure for TCP source without password reference")
	}
	if !strings.Contains(err.Error(), "password") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSourceAllowsSocketWithoutPassword(t *testing.T) {
	cfg := baseConfig()
	cfg.Host = ""
	cfg.Socket = "/var/run/mysqld/mysqld.sock"
	d := New(&stubRunner{}, cfg)
	if err := d.ValidateSource(context.Background(), domain.Source{}); err != nil {
		t.Fatalf("socket auth should not require a password reference: %v", err)
	}
}

func TestDumpArgsCarryUsernameAndFlags(t *testing.T) {
	cfg := baseConfig()
	cfg.SingleTransaction = true
	cfg.Quick = true
	cfg.Routines = true
	cfg.Triggers = true
	cfg.Events = true
	cfg.ExcludeTables = []string{"cache", "db2.sessions"}
	cfg.IncludeDatabases = []string{"app", "analytics"}
	cfg.ExtraOptions = []string{"--max-allowed-packet=512M"}

	d := New(&stubRunner{}, cfg)
	args := d.dumpArgs()
	argsStr := strings.Join(args, " ")

	for _, expected := range []string{
		"--user backup",
		"--host 127.0.0.1",
		"--port 3306",
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"--hex-blob",
		"--ignore-table=app.cache",
		"--ignore-table=db2.sessions",
		"--databases app analytics",
		"--max-allowed-packet=512M",
	} {
		if !strings.Contains(argsStr, expected) {
			t.Fatalf("dumpArgs %q missing expected part: %q", argsStr, expected)
		}
	}
}

func TestClientArgs(t *testing.T) {
	cfg := baseConfig()
	cfg.ExtraOptions = []string{"--default-character-set=utf8mb4"}
	d := New(&stubRunner{}, cfg)
	args := d.clientArgs()
	argsStr := strings.Join(args, " ")

	for _, expected := range []string{
		"--user backup",
		"--host 127.0.0.1",
		"--port 3306",
		"--default-character-set=utf8mb4",
		"app",
	} {
		if !strings.Contains(argsStr, expected) {
			t.Fatalf("clientArgs %q missing %q", argsStr, expected)
		}
	}
}

type stubSource struct {
	content []byte
}

func (s *stubSource) OpenArtifact(ctx context.Context, id domain.ArtifactID) (io.ReadCloser, domain.BackupArtifact, error) {
	return io.NopCloser(bytes.NewReader(s.content)), domain.BackupArtifact{ID: id, Name: "database.sql"}, nil
}

func TestRestore(t *testing.T) {
	t.Run("successful mysql restore", func(t *testing.T) {
		var capturedReq ports.ProcessRequest
		runner := &stubRunner{
			runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
				capturedReq = req
				return ports.ProcessResult{ExitCode: 0}, nil
			},
		}
		cfg := baseConfig()
		d := New(runner, cfg)
		src := &stubSource{content: []byte("CREATE TABLE test (id INT);")}
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
		if !res.Succeeded || res.Engine != domain.EngineMySQL {
			t.Fatalf("unexpected restore result: %+v", res)
		}
		if capturedReq.Executable != "mysql" {
			t.Fatalf("expected executable mysql, got %s", capturedReq.Executable)
		}
	})

	t.Run("mariadb dialect restore", func(t *testing.T) {
		var capturedReq ports.ProcessRequest
		runner := &stubRunner{
			runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
				capturedReq = req
				return ports.ProcessResult{ExitCode: 0}, nil
			},
		}
		cfg := baseConfig()
		cfg.Engine = "mariadb"
		d := New(runner, cfg)
		src := &stubSource{content: []byte("CREATE TABLE test (id INT);")}
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
		if !res.Succeeded || res.Engine != domain.EngineMariaDB {
			t.Fatalf("unexpected restore result: %+v", res)
		}
		if capturedReq.Executable != "mariadb" {
			t.Fatalf("expected executable mariadb, got %s", capturedReq.Executable)
		}
	})

	t.Run("failed restore returns ErrRestoreToolFailed", func(t *testing.T) {
		runner := &stubRunner{
			runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
				return ports.ProcessResult{ExitCode: 1, StdErr: "ERROR 1049 (42000): Unknown database 'app'"}, nil
			},
		}
		d := New(runner, baseConfig())
		src := &stubSource{content: []byte("CREATE TABLE test (id INT);")}
		req := ports.RestoreRequest{
			Plan: domain.RestorePlan{
				Artifacts: []domain.ArtifactID{"art1"},
			},
		}
		_, err := d.Restore(context.Background(), req, src)
		if err == nil {
			t.Fatal("expected restore failure")
		}
		appErr, ok := err.(*domain.AppError)
		if !ok || appErr.Code != domain.ErrRestoreToolFailed {
			t.Fatalf("expected ErrRestoreToolFailed, got %v", err)
		}
	})
}

func TestParseVersion(t *testing.T) {
	v1 := parseVersion("mysqldump  Ver 8.0.33 for Linux on x86_64 (Source distribution)")
	if v1.Major != 8 || v1.Minor != 0 || v1.Patch != 33 {
		t.Fatalf("expected 8.0.33, got %+v", v1)
	}

	v2 := parseVersion("mariadb-dump  Ver 10.11.4-MariaDB for debian-linux-gnu on x86_64")
	if v2.Major != 10 || v2.Minor != 11 || v2.Patch != 4 {
		t.Fatalf("expected 10.11.4, got %+v", v2)
	}
}

func TestSnapshotSource(t *testing.T) {
	runner := &stubRunner{
		runFn: func(req ports.ProcessRequest) (ports.ProcessResult, error) {
			_, _ = req.StandardOutput.Write([]byte("mock-mysql-dump-sql"))
			return ports.ProcessResult{ExitCode: 0}, nil
		},
	}
	cfg := baseConfig()
	cfg.Socket = "/var/run/mysqld/mysqld.sock"
	cfg.Host = ""
	d := New(runner, cfg)
	snapSrc := NewSnapshotSource(d)

	if snapSrc.Name() != "mysql" {
		t.Fatalf("expected mysql, got %s", snapSrc.Name())
	}
	if err := snapSrc.Validate(context.Background(), domain.Source{}); err != nil {
		t.Fatal(err)
	}

	scratch := t.TempDir()
	art, err := snapSrc.CreateSnapshot(context.Background(), ports.SnapshotRequest{
		Source:           domain.Source{ID: "src-1"},
		ScratchDirectory: scratch,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer art.Close()

	if art.Size() != int64(len("mock-mysql-dump-sql")) {
		t.Fatalf("expected size %d, got %d", len("mock-mysql-dump-sql"), art.Size())
	}
}
