package mysql

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type stubRunner struct{}

func (stubRunner) Run(context.Context, ports.ProcessRequest) (ports.ProcessResult, error) {
	return ports.ProcessResult{ExitCode: 0}, nil
}
func (stubRunner) Start(context.Context, ports.ProcessRequest) (ports.RunningProcess, error) {
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
	d := New(stubRunner{}, cfg)
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
}

func TestPasswordResolvesFromNamedEnvVar(t *testing.T) {
	t.Setenv("DBVAULT_TEST_MYSQL_PWD", "s3cret-from-env")
	cfg := baseConfig()
	cfg.Password = SecretReference{Env: "DBVAULT_TEST_MYSQL_PWD"}
	d := New(stubRunner{}, cfg)
	envVars := d.env()
	v, ok := findEnv(envVars, "MYSQL_PWD")
	if !ok || v.Value != "s3cret-from-env" {
		t.Fatalf("env=%+v ok=%v", envVars, ok)
	}
}

func TestValidateSourceRequiresPasswordOnTCP(t *testing.T) {
	cfg := baseConfig()
	cfg.Password = SecretReference{}
	d := New(stubRunner{}, cfg)
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
	d := New(stubRunner{}, cfg)
	if err := d.ValidateSource(context.Background(), domain.Source{}); err != nil {
		t.Fatalf("socket auth should not require a password reference: %v", err)
	}
}

func TestDumpArgsCarryUsername(t *testing.T) {
	d := New(stubRunner{}, baseConfig())
	args := d.dumpArgs()
	found := false
	for i, a := range args {
		if a == "--user" && i+1 < len(args) && args[i+1] == "backup" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dump args missing --user backup: %v", args)
	}
}
