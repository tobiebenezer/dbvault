//go:build restricted

package bootstrap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"

	catsqlite "github.com/dbvault/dbvault/internal/adapters/catalogue/sqlite"
	zstdc "github.com/dbvault/dbvault/internal/adapters/compression/zstd"
	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	"github.com/dbvault/dbvault/internal/adapters/id"
	"github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/adapters/scratch"
	srcsqlite "github.com/dbvault/dbvault/internal/adapters/source/sqlite"
	"github.com/dbvault/dbvault/internal/adapters/storage/filesystem"
	"github.com/dbvault/dbvault/internal/adapters/storage/s3"
	"github.com/dbvault/dbvault/internal/application/backup"
	"github.com/dbvault/dbvault/internal/application/restore"
	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Application struct {
	Config    config.Config
	Binding   config.RuntimeBinding
	Catalogue *catsqlite.Catalogue
	Backup    *backup.Service
	Restore   *restore.Service
	Close     func(context.Context) error
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
	key := sha256.Sum256([]byte(binding.Repository.ID + ":" + binding.Repository.Encryption.ActiveKey))
	enc, err := aead.New(key[:])
	if err != nil {
		_ = cat.Close()
		return nil, err
	}
	signer, err := ed25519.Generate(firstNonEmpty(binding.Repository.Signing.KeyID, "restricted-signing"))
	if err != nil {
		_ = cat.Close()
		return nil, err
	}
	store, err := buildRestrictedStore(binding.Primary)
	if err != nil {
		_ = cat.Close()
		return nil, err
	}
	if err := store.Validate(ctx); err != nil {
		_ = cat.Close()
		return nil, err
	}
	if binding.Source.Engine != "sqlite" || binding.Source.SQLite == nil {
		_ = cat.Close()
		return nil, fmt.Errorf("restricted standalone runtime currently executes SQLite sources; selected source %s uses %s", binding.Source.ID, binding.Source.Engine)
	}
	repo := buildRepository(binding.Repository, binding.Primary.ID)
	chunkBytes, _ := config.ParseBytes(binding.Repository.Chunking.SQLite.TargetSize)
	comp := zstdc.New(binding.Repository.Compression.Level, binding.Repository.Compression.MinimumSavingsPercent)
	bsvc := &backup.Service{Catalogue: cat, Source: srcsqlite.New("sqlite3"), Store: store, Scratch: scratch.New(cfg.Server.ScratchDirectory), Compressor: comp, Encryptor: enc, Signer: signer, Clock: ports.SystemClock{}, IDs: id.Generator{}, DedupKey: key[:], Repository: repo, TargetChunkBytes: chunkBytes}
	rsvc := &restore.Service{Catalogue: cat, Store: store, Compressor: comp, Encryptor: enc, Signer: signer}
	return &Application{Config: cfg, Binding: binding, Catalogue: cat, Backup: bsvc, Restore: rsvc, Close: func(context.Context) error { return cat.Close() }}, nil
}

func buildRestrictedStore(d config.DestinationConfig) (ports.ObjectStore, error) {
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
		return s3.New(s3.Config{Endpoint: d.S3.Endpoint, Region: d.S3.Region, Bucket: d.S3.Bucket, Prefix: d.S3.Prefix, UsePathStyle: d.S3.Addressing.PathStyle}, d.Profile), nil
	default:
		return nil, domain.NewError(domain.ErrConfigurationInvalid, "unknown destination driver", nil)
	}
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

func firstNonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
