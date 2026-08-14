package config

import (
	"strings"
	"testing"
)

func validGraph() Config {
	enabled := true
	return Config{Version: 7, Server: Server{DataDirectory: "./data", ScratchDirectory: "./scratch", ShutdownGrace: "30s"}, Catalogue: Catalogue{Driver: "sqlite", Path: "./data/catalogue.sqlite"}, SecretProviders: []SecretProviderConfig{{ID: "files", Driver: "file", File: &FileSecretProvider{Root: "./secrets"}}}, Destinations: []DestinationConfig{{ID: "fs", Driver: "filesystem", Filesystem: &FilesystemDestinationConfig{Root: "./repo"}}}, Repositories: []RepositoryConfig{{ID: "repo", Mode: "single", Primary: &RepositoryPrimary{Destination: "fs"}, Encryption: RepositoryEncryptionConfig{KeyProvider: "files", ActiveKey: "key"}, Signing: RepositorySigningConfig{KeyProvider: "files", KeyID: "sign"}, Retention: RetentionConfig{MinimumVerifiedSnapshots: 2}}}, Sources: []SourceConfig{{ID: "source", Engine: "sqlite", Repository: "repo", Enabled: &enabled, SQLite: &SQLiteSourceConfig{Path: "/tmp/source.sqlite"}}}}
}

func TestUnknownRepositoryReferenceFails(t *testing.T) {
	cfg := validGraph()
	cfg.Sources[0].Repository = "missing"
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "unknown repository") {
		t.Fatalf("expected unknown repository error, got %v", err)
	}
}

func TestBindingResolvesSourceRepositoryDestination(t *testing.T) {
	cfg := validGraph()
	cfg.Normalize()
	binding, err := cfg.Binding("source")
	if err != nil {
		t.Fatal(err)
	}
	if binding.Repository.ID != "repo" || binding.Primary.ID != "fs" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
}

func TestEffectiveConfigurationRedactsLiteral(t *testing.T) {
	cfg := validGraph()
	cfg.Destinations = []DestinationConfig{{ID: "s3", Driver: "s3", S3: &S3DestinationConfig{Endpoint: "https://example.com", Bucket: "bucket", Credentials: S3CredentialConfig{AccessKeyID: SecretReference{Literal: "access"}, SecretAccessKey: SecretReference{Literal: "secret"}}}}}
	cfg.Repositories[0].Primary.Destination = "s3"
	b, err := MarshalRedactedJSON(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"literal": "access"`) || strings.Contains(string(b), `"literal": "secret"`) {
		t.Fatal("literal secret was not redacted")
	}
}

func TestProductionModeRejectsLiteralSecrets(t *testing.T) {
	cfg := validGraph()
	cfg.ControlPlane.Mode = "production"
	cfg.Destinations = []DestinationConfig{{ID: "s3", Driver: "s3", S3: &S3DestinationConfig{Endpoint: "https://example.com", Bucket: "bucket", Credentials: S3CredentialConfig{AccessKeyID: SecretReference{Literal: "access"}, SecretAccessKey: SecretReference{Literal: "secret"}}}}}
	cfg.Repositories[0].Primary.Destination = "s3"
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "literal secrets") {
		t.Fatalf("expected literal secret rejection, got %v", err)
	}
}

func TestDayDuration(t *testing.T) {
	duration, err := ParseDuration("7d")
	if err != nil {
		t.Fatal(err)
	}
	if duration.Hours() != 168 {
		t.Fatalf("hours=%v", duration.Hours())
	}
}
