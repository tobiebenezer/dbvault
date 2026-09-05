//go:build !restricted

package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	catsqlite "github.com/dbvault/dbvault/internal/adapters/catalogue/sqlite"
	zstdc "github.com/dbvault/dbvault/internal/adapters/compression/zstd"
	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	"github.com/dbvault/dbvault/internal/adapters/id"
	filekeys "github.com/dbvault/dbvault/internal/adapters/keys/file"
	edsigner "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/adapters/scratch"
	"github.com/dbvault/dbvault/internal/adapters/storage/filesystem"
	"github.com/dbvault/dbvault/internal/adapters/storage/s3"
	"github.com/dbvault/dbvault/internal/application/backup"
	"github.com/dbvault/dbvault/internal/application/restore"
	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Application struct {
	Config      config.Config
	Binding     config.RuntimeBinding
	Catalogue   *catsqlite.Catalogue
	ObjectStore ports.ObjectStore
	Backup      *backup.Service
	Restore     *restore.Service
	Close       func(context.Context) error
}

func Build(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Application, error) {
	cfg.Normalize()
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	binding, err := cfg.Binding("")
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if err := os.MkdirAll(cfg.Server.DataDirectory, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Server.ScratchDirectory, 0700); err != nil {
		return nil, err
	}
	cat, err := catsqlite.Open(cfg.Catalogue.Path)
	if err != nil {
		return nil, err
	}
	keyDir, err := keyProviderRoot(cfg, binding.Repository.Encryption.KeyProvider)
	if err != nil {
		_ = cat.Close()
		return nil, err
	}
	keys := filekeys.New(keyDir)
	material, err := keys.EncryptionKey(binding.Repository.ID, binding.Repository.Encryption.ActiveKey)
	if err != nil {
		_ = cat.Close()
		return nil, domain.NewError(domain.ErrConfigurationInvalid, "load repository encryption key; run dbvault key generate first", err)
	}
	pair, err := keys.Signing(binding.Repository.ID)
	if err != nil {
		_ = cat.Close()
		return nil, domain.NewError(domain.ErrConfigurationInvalid, "load repository signing key; run dbvault key generate first", err)
	}
	enc, err := aead.New(material.Secret)
	if err != nil {
		_ = cat.Close()
		return nil, err
	}
	signer := edsigner.New(pair.ID, pair.Private, pair.Public)
	store, err := buildProductionStore(cfg, binding.Primary)
	if err != nil {
		_ = cat.Close()
		return nil, err
	}
	if err := store.Validate(ctx); err != nil {
		_ = cat.Close()
		return nil, err
	}
	srcDriver, err := buildSourceDriver(binding.Source)
	if err != nil {
		_ = cat.Close()
		return nil, err
	}
	repo := buildRepository(binding.Repository, binding.Primary.ID)
	chunkBytes, _ := config.ParseBytes(binding.Repository.Chunking.SQLite.TargetSize)
	comp := zstdc.New(binding.Repository.Compression.Level, binding.Repository.Compression.MinimumSavingsPercent)
	bsvc := &backup.Service{Catalogue: cat, Source: srcDriver, Store: store, Scratch: scratch.New(cfg.Server.ScratchDirectory), Compressor: comp, Encryptor: enc, Signer: signer, Clock: ports.SystemClock{}, IDs: id.Generator{}, DedupKey: material.Secret, Repository: repo, TargetChunkBytes: chunkBytes}
	rsvc := &restore.Service{Catalogue: cat, Store: store, Compressor: comp, Encryptor: enc, Signer: signer, DedupKey: material.Secret}
	return &Application{Config: cfg, Binding: binding, Catalogue: cat, ObjectStore: store, Backup: bsvc, Restore: rsvc, Close: func(context.Context) error { return cat.Close() }}, nil
}

func keyProviderRoot(cfg config.Config, id string) (string, error) {
	if id == "" {
		id = "local-files"
	}
	for _, p := range cfg.SecretProviders {
		if p.ID == id && p.Driver == "file" && p.File != nil {
			return p.File.Root, nil
		}
	}
	return "", fmt.Errorf("file key provider %q not found", id)
}

func buildProductionStore(cfg config.Config, d config.DestinationConfig) (ports.ObjectStore, error) {
	switch d.Driver {
	case "filesystem":
		if d.Filesystem == nil {
			return nil, fmt.Errorf("filesystem destination %s is incomplete", d.ID)
		}
		return filesystem.New(d.Filesystem.Root), nil
	case "s3":
		if d.S3 == nil {
			return nil, fmt.Errorf("s3 destination %s is incomplete", d.ID)
		}
		access, err := secretLocatorOrValue(cfg, d.S3.Credentials.AccessKeyID)
		if err != nil {
			return nil, err
		}
		secret, err := secretLocatorOrValue(cfg, d.S3.Credentials.SecretAccessKey)
		if err != nil {
			return nil, err
		}
		return s3.New(s3.Config{Endpoint: d.S3.Endpoint, Region: d.S3.Region, Bucket: d.S3.Bucket, Prefix: d.S3.Prefix, AccessKeyID: access, SecretAccessKey: secret, UsePathStyle: d.S3.Addressing.PathStyle}, d.Profile), nil
	default:
		return nil, domain.NewError(domain.ErrConfigurationInvalid, "unknown destination driver", nil)
	}
}

func secretLocatorOrValue(cfg config.Config, r config.SecretReference) (string, error) {
	if r.Env != "" {
		return "env:" + r.Env, nil
	}
	if r.File != "" {
		return "file:" + r.File, nil
	}
	if r.Provider != "" {
		for _, p := range cfg.SecretProviders {
			if p.ID == r.Provider && p.File != nil {
				return "file:" + p.File.Root + "/" + r.Key, nil
			}
		}
	}
	return cfg.ResolveSecret(r, true)
}

func buildRepository(r config.RepositoryConfig, primary string) domain.Repository {
	mode := domain.RepositorySingle
	switch r.Mode {
	case "mirror":
		mode = domain.RepositoryMirror
	case "primary_replica":
		mode = domain.RepositoryPrimaryReplica
	}
	budget, _ := config.ParseBytes(r.Budget.MaximumPhysicalBytes)
	return domain.Repository{ID: domain.RepositoryID(r.ID), Name: r.ID, Mode: mode, PrimaryDestination: domain.DestinationID(primary), Encryption: domain.EncryptionPolicy{Algorithm: "aes-256-gcm", KeyID: r.Encryption.ActiveKey}, Retention: domain.RetentionPolicy{KeepLast: r.Retention.KeepLast, Daily: r.Retention.Daily, Weekly: r.Retention.Weekly, Monthly: r.Retention.Monthly, MinimumVerifiedSnapshots: r.Retention.MinimumVerifiedSnapshots}, Budget: domain.BudgetPolicy{MaximumRepositoryBytes: budget}}
}
