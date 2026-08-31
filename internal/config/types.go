package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const CurrentVersion = 7

type Config struct {
	Version         int                    `json:"version" yaml:"version"`
	Server          Server                 `json:"server" yaml:"server"`
	Catalogue       Catalogue              `json:"catalogue" yaml:"catalogue"`
	SecretProviders []SecretProviderConfig `json:"secret_providers,omitempty" yaml:"secret_providers,omitempty"`
	Destinations    []DestinationConfig    `json:"destinations,omitempty" yaml:"destinations,omitempty"`
	Repositories    []RepositoryConfig     `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	Sources         []SourceConfig         `json:"sources,omitempty" yaml:"sources,omitempty"`
	Schedules       []ScheduleConfig       `json:"schedules,omitempty" yaml:"schedules,omitempty"`
	Verification    VerificationConfig     `json:"verification,omitempty" yaml:"verification,omitempty"`
	Notifications   NotificationConfig     `json:"notifications,omitempty" yaml:"notifications,omitempty"`
	Metrics         MetricsConfig          `json:"metrics,omitempty" yaml:"metrics,omitempty"`
	Security        SecurityConfig         `json:"security,omitempty" yaml:"security,omitempty"`
	Doctor          DoctorConfig           `json:"doctor,omitempty" yaml:"doctor,omitempty"`
	ControlPlane    ControlPlaneConfig     `json:"control_plane,omitempty" yaml:"control_plane,omitempty"`

	// Legacy Phase 2 fields are retained for config migration only.
	Source      Source      `json:"source,omitempty" yaml:"source,omitempty"`
	Repository  Repository  `json:"repository,omitempty" yaml:"repository,omitempty"`
	Destination Destination `json:"destination,omitempty" yaml:"destination,omitempty"`
	Keys        Keys        `json:"keys,omitempty" yaml:"keys,omitempty"`
	Schedule    Schedule    `json:"schedule,omitempty" yaml:"schedule,omitempty"`
}

type Server struct {
	DataDirectory     string         `json:"data_directory" yaml:"data_directory"`
	ScratchDirectory  string         `json:"scratch_directory" yaml:"scratch_directory"`
	LogSpoolDirectory string         `json:"log_spool_directory,omitempty" yaml:"log_spool_directory,omitempty"`
	ShutdownGrace     string         `json:"shutdown_grace,omitempty" yaml:"shutdown_grace,omitempty"`
	Listen            string         `json:"listen,omitempty" yaml:"listen,omitempty"`
	API               ListenerConfig `json:"api,omitempty" yaml:"api,omitempty"`
	Metrics           ListenerConfig `json:"metrics,omitempty" yaml:"metrics,omitempty"`
}

type ListenerConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Listen  string `json:"listen" yaml:"listen"`
}

type Catalogue struct {
	Driver string          `json:"driver,omitempty" yaml:"driver,omitempty"`
	Path   string          `json:"path" yaml:"path"`
	SQLite CatalogueSQLite `json:"sqlite,omitempty" yaml:"sqlite,omitempty"`
}

type CatalogueSQLite struct {
	JournalMode string `json:"journal_mode,omitempty" yaml:"journal_mode,omitempty"`
	Synchronous string `json:"synchronous,omitempty" yaml:"synchronous,omitempty"`
	BusyTimeout string `json:"busy_timeout,omitempty" yaml:"busy_timeout,omitempty"`
}

type SecretProviderConfig struct {
	ID     string              `json:"id" yaml:"id"`
	Driver string              `json:"driver" yaml:"driver"`
	File   *FileSecretProvider `json:"file,omitempty" yaml:"file,omitempty"`
}

type FileSecretProvider struct {
	Root           string `json:"root" yaml:"root"`
	RejectSymlinks bool   `json:"reject_symlinks" yaml:"reject_symlinks"`
	RequireMode    string `json:"require_mode,omitempty" yaml:"require_mode,omitempty"`
}

type SecretReference struct {
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Key      string `json:"key,omitempty" yaml:"key,omitempty"`
	Env      string `json:"env,omitempty" yaml:"env,omitempty"`
	File     string `json:"file,omitempty" yaml:"file,omitempty"`
	Literal  string `json:"literal,omitempty" yaml:"literal,omitempty"`
}

func (r SecretReference) Empty() bool {
	return r.Provider == "" && r.Key == "" && r.Env == "" && r.File == "" && r.Literal == ""
}

type DestinationConfig struct {
	ID         string                       `json:"id" yaml:"id"`
	Driver     string                       `json:"driver" yaml:"driver"`
	Profile    string                       `json:"profile,omitempty" yaml:"profile,omitempty"`
	S3         *S3DestinationConfig         `json:"s3,omitempty" yaml:"s3,omitempty"`
	Filesystem *FilesystemDestinationConfig `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
}

type S3DestinationConfig struct {
	Endpoint        string                 `json:"endpoint" yaml:"endpoint"`
	Region          string                 `json:"region" yaml:"region"`
	Bucket          string                 `json:"bucket" yaml:"bucket"`
	Prefix          string                 `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Addressing      S3AddressingConfig     `json:"addressing,omitempty" yaml:"addressing,omitempty"`
	Credentials     S3CredentialConfig     `json:"credentials" yaml:"credentials"`
	Transport       S3TransportConfig      `json:"transport,omitempty" yaml:"transport,omitempty"`
	Multipart       MultipartConfig        `json:"multipart,omitempty" yaml:"multipart,omitempty"`
	TLS             TLSConfig              `json:"tls,omitempty" yaml:"tls,omitempty"`
	CapabilityHints StorageCapabilityHints `json:"capability_hints,omitempty" yaml:"capability_hints,omitempty"`
}

type S3AddressingConfig struct {
	PathStyle bool `json:"path_style" yaml:"path_style"`
}
type S3CredentialConfig struct {
	AccessKeyID     SecretReference `json:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey SecretReference `json:"secret_access_key" yaml:"secret_access_key"`
	SessionToken    SecretReference `json:"session_token,omitempty" yaml:"session_token,omitempty"`
}
type S3TransportConfig struct {
	RequestTimeout  string `json:"request_timeout,omitempty" yaml:"request_timeout,omitempty"`
	MaximumAttempts int    `json:"maximum_attempts,omitempty" yaml:"maximum_attempts,omitempty"`
}
type MultipartConfig struct {
	Threshold   string `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	PartSize    string `json:"part_size,omitempty" yaml:"part_size,omitempty"`
	Concurrency int    `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
}
type TLSConfig struct {
	AllowInsecureHTTP bool   `json:"allow_insecure_http,omitempty" yaml:"allow_insecure_http,omitempty"`
	CAFile            string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`
	CertFile          string `json:"cert_file,omitempty" yaml:"cert_file,omitempty"`
	KeyFile           string `json:"key_file,omitempty" yaml:"key_file,omitempty"`
}
type StorageCapabilityHints struct {
	ConditionalCreate *bool `json:"conditional_create,omitempty" yaml:"conditional_create,omitempty"`
	Multipart         *bool `json:"multipart,omitempty" yaml:"multipart,omitempty"`
	BatchDelete       *bool `json:"batch_delete,omitempty" yaml:"batch_delete,omitempty"`
}
type FilesystemDestinationConfig struct {
	Root          string `json:"root" yaml:"root"`
	FileMode      string `json:"file_mode,omitempty" yaml:"file_mode,omitempty"`
	DirectoryMode string `json:"directory_mode,omitempty" yaml:"directory_mode,omitempty"`
}

type RepositoryConfig struct {
	ID          string                      `json:"id" yaml:"id"`
	Mode        string                      `json:"mode" yaml:"mode"`
	Primary     *RepositoryPrimary          `json:"primary,omitempty" yaml:"primary,omitempty"`
	Mirrors     []RepositoryDestination     `json:"mirrors,omitempty" yaml:"mirrors,omitempty"`
	Replicas    []RepositoryReplica         `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	Encryption  RepositoryEncryptionConfig  `json:"encryption" yaml:"encryption"`
	Signing     RepositorySigningConfig     `json:"signing" yaml:"signing"`
	Compression RepositoryCompressionConfig `json:"compression,omitempty" yaml:"compression,omitempty"`
	Chunking    RepositoryChunkingConfig    `json:"chunking,omitempty" yaml:"chunking,omitempty"`
	Retention   RetentionConfig             `json:"retention" yaml:"retention"`
	Budget      BudgetConfig                `json:"budget" yaml:"budget"`
}

type RepositoryPrimary struct {
	Destination string `json:"destination" yaml:"destination"`
}
type RepositoryDestination struct {
	Destination string `json:"destination" yaml:"destination"`
	Required    bool   `json:"required" yaml:"required"`
}
type RepositoryReplica struct {
	Destination    string `json:"destination" yaml:"destination"`
	RequiredWithin string `json:"required_within,omitempty" yaml:"required_within,omitempty"`
}
type RepositoryEncryptionConfig struct {
	KeyProvider string `json:"key_provider" yaml:"key_provider"`
	ActiveKey   string `json:"active_key" yaml:"active_key"`
}
type RepositorySigningConfig struct {
	KeyProvider string `json:"key_provider" yaml:"key_provider"`
	KeyID       string `json:"key_id" yaml:"key_id"`
}
type RepositoryCompressionConfig struct {
	Algorithm             string `json:"algorithm,omitempty" yaml:"algorithm,omitempty"`
	Level                 int    `json:"level,omitempty" yaml:"level,omitempty"`
	MinimumSavingsPercent int    `json:"minimum_savings_percent,omitempty" yaml:"minimum_savings_percent,omitempty"`
}
type RepositoryChunkingConfig struct {
	SQLite  ChunkStrategyConfig `json:"sqlite,omitempty" yaml:"sqlite,omitempty"`
	Streams ChunkStrategyConfig `json:"streams,omitempty" yaml:"streams,omitempty"`
}
type ChunkStrategyConfig struct {
	Strategy   string `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	TargetSize string `json:"target_size,omitempty" yaml:"target_size,omitempty"`
}
type RetentionConfig struct {
	KeepLast                 int    `json:"keep_last" yaml:"keep_last"`
	Daily                    int    `json:"daily" yaml:"daily"`
	Weekly                   int    `json:"weekly" yaml:"weekly"`
	Monthly                  int    `json:"monthly" yaml:"monthly"`
	MinimumVerifiedSnapshots int    `json:"minimum_verified_snapshots,omitempty" yaml:"minimum_verified_snapshots,omitempty"`
	TombstoneGrace           string `json:"tombstone_grace,omitempty" yaml:"tombstone_grace,omitempty"`
	GCSweepInterval          string `json:"gc_sweep_interval,omitempty" yaml:"gc_sweep_interval,omitempty"`
}
type BudgetConfig struct {
	MaximumPhysicalBytes string `json:"maximum_physical_bytes,omitempty" yaml:"maximum_physical_bytes,omitempty"`
	ReservePercent       int    `json:"reserve_percent,omitempty" yaml:"reserve_percent,omitempty"`
	LogReserveHorizon    string `json:"log_reserve_horizon,omitempty" yaml:"log_reserve_horizon,omitempty"`
}

type SourceConfig struct {
	ID         string                `json:"id" yaml:"id"`
	Engine     string                `json:"engine" yaml:"engine"`
	Repository string                `json:"repository" yaml:"repository"`
	Enabled    *bool                 `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	SQLite     *SQLiteSourceConfig   `json:"sqlite,omitempty" yaml:"sqlite,omitempty"`
	Postgres   *PostgresSourceConfig `json:"postgres,omitempty" yaml:"postgres,omitempty"`
	MySQL      *MySQLSourceConfig    `json:"mysql,omitempty" yaml:"mysql,omitempty"`
}

func (s SourceConfig) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// SourceByID returns the configured source with the given ID; an empty ID
// matches the first configured source.
func SourceByID(c Config, id string) (SourceConfig, bool) {
	for i := range c.Sources {
		if id == "" || c.Sources[i].ID == id {
			return c.Sources[i], true
		}
	}
	return SourceConfig{}, false
}

type SQLiteSourceConfig struct {
	Path         string `json:"path" yaml:"path"`
	BusyTimeout  string `json:"busy_timeout,omitempty" yaml:"busy_timeout,omitempty"`
	PagesPerStep int    `json:"pages_per_step,omitempty" yaml:"pages_per_step,omitempty"`
}
type PostgresSourceConfig struct {
	Host             string                       `json:"host" yaml:"host"`
	Port             int                          `json:"port,omitempty" yaml:"port,omitempty"`
	Database         string                       `json:"database" yaml:"database"`
	Username         string                       `json:"username" yaml:"username"`
	Password         SecretReference              `json:"password" yaml:"password"`
	SSL              DatabaseTLSConfig            `json:"ssl,omitempty" yaml:"ssl,omitempty"`
	ConnectTimeout   string                       `json:"connect_timeout,omitempty" yaml:"connect_timeout,omitempty"`
	StatementTimeout string                       `json:"statement_timeout,omitempty" yaml:"statement_timeout,omitempty"`
	LockWaitTimeout  string                       `json:"lock_wait_timeout,omitempty" yaml:"lock_wait_timeout,omitempty"`
	LogicalBackup    PostgresLogicalBackupConfig  `json:"logical_backup,omitempty" yaml:"logical_backup,omitempty"`
	PhysicalBackup   PostgresPhysicalBackupConfig `json:"physical_backup,omitempty" yaml:"physical_backup,omitempty"`
	WAL              PostgresWALConfig            `json:"wal,omitempty" yaml:"wal,omitempty"`
}
type DatabaseTLSConfig struct {
	Mode       string `json:"mode,omitempty" yaml:"mode,omitempty"`
	RootCert   string `json:"root_cert,omitempty" yaml:"root_cert,omitempty"`
	ClientCert string `json:"client_cert,omitempty" yaml:"client_cert,omitempty"`
	ClientKey  string `json:"client_key,omitempty" yaml:"client_key,omitempty"`
	CAFile     string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`
	CertFile   string `json:"cert_file,omitempty" yaml:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty" yaml:"key_file,omitempty"`
}
type PostgresLogicalBackupConfig struct {
	Enabled           bool   `json:"enabled" yaml:"enabled"`
	Format            string `json:"format,omitempty" yaml:"format,omitempty"`
	IncludeGlobals    bool   `json:"include_globals,omitempty" yaml:"include_globals,omitempty"`
	IncludeOwnership  bool   `json:"include_ownership,omitempty" yaml:"include_ownership,omitempty"`
	IncludePrivileges bool   `json:"include_privileges,omitempty" yaml:"include_privileges,omitempty"`
	ParallelJobs      int    `json:"parallel_jobs,omitempty" yaml:"parallel_jobs,omitempty"`
}
type PostgresPhysicalBackupConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Strategy  string `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	FullEvery string `json:"full_every,omitempty" yaml:"full_every,omitempty"`
}
type PostgresWALConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	Mode               string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Slot               string `json:"slot,omitempty" yaml:"slot,omitempty"`
	PublicationTimeout string `json:"publication_timeout,omitempty" yaml:"publication_timeout,omitempty"`
}
type MySQLSourceConfig struct {
	Host           string                   `json:"host" yaml:"host"`
	Port           int                      `json:"port,omitempty" yaml:"port,omitempty"`
	Database       string                   `json:"database" yaml:"database"`
	Username       string                   `json:"username" yaml:"username"`
	Password       SecretReference          `json:"password" yaml:"password"`
	TLS            DatabaseTLSConfig        `json:"tls,omitempty" yaml:"tls,omitempty"`
	ConnectTimeout string                   `json:"connect_timeout,omitempty" yaml:"connect_timeout,omitempty"`
	LogicalBackup  MySQLLogicalBackupConfig `json:"logical_backup,omitempty" yaml:"logical_backup,omitempty"`
	Binlog         MySQLBinlogConfig        `json:"binlog,omitempty" yaml:"binlog,omitempty"`
}
type MySQLLogicalBackupConfig struct {
	Enabled                bool   `json:"enabled" yaml:"enabled"`
	SingleTransaction      bool   `json:"single_transaction,omitempty" yaml:"single_transaction,omitempty"`
	Quick                  bool   `json:"quick,omitempty" yaml:"quick,omitempty"`
	Routines               bool   `json:"routines,omitempty" yaml:"routines,omitempty"`
	Triggers               bool   `json:"triggers,omitempty" yaml:"triggers,omitempty"`
	Events                 bool   `json:"events,omitempty" yaml:"events,omitempty"`
	NonTransactionalPolicy string `json:"non_transactional_policy,omitempty" yaml:"non_transactional_policy,omitempty"`
}
type MySQLBinlogConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`
	GTID    string `json:"gtid,omitempty" yaml:"gtid,omitempty"`
}

type ScheduleConfig struct {
	ID           string `json:"id" yaml:"id"`
	Source       string `json:"source" yaml:"source"`
	Operation    string `json:"operation" yaml:"operation"`
	Cron         string `json:"cron" yaml:"cron"`
	Timezone     string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	Enabled      *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	EverySeconds int    `json:"every_seconds,omitempty" yaml:"every_seconds,omitempty"`
}
type VerificationConfig struct {
	MetadataInterval string             `json:"metadata_interval,omitempty" yaml:"metadata_interval,omitempty"`
	SampleInterval   string             `json:"sample_interval,omitempty" yaml:"sample_interval,omitempty"`
	FullInterval     string             `json:"full_interval,omitempty" yaml:"full_interval,omitempty"`
	RestoreDrills    RestoreDrillConfig `json:"restore_drills,omitempty" yaml:"restore_drills,omitempty"`
}
type RestoreDrillConfig struct {
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	Schedule string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	Timezone string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}
type NotificationConfig struct {
	Webhook WebhookNotificationConfig `json:"webhook,omitempty" yaml:"webhook,omitempty"`
}
type WebhookNotificationConfig struct {
	Enabled       bool            `json:"enabled" yaml:"enabled"`
	URL           SecretReference `json:"url,omitempty" yaml:"url,omitempty"`
	SigningSecret SecretReference `json:"signing_secret,omitempty" yaml:"signing_secret,omitempty"`
}
type MetricsConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
}
type SecurityConfig struct {
	AllowMasterKeyReveal bool `json:"allow_master_key_reveal" yaml:"allow_master_key_reveal"`
}
type DoctorConfig struct {
	MinFreeBytes int64 `json:"min_free_bytes,omitempty" yaml:"min_free_bytes,omitempty"`
}
type ControlPlaneConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	Mode               string `json:"mode,omitempty" yaml:"mode,omitempty"`
	OrganisationID     string `json:"organisation_id,omitempty" yaml:"organisation_id,omitempty"`
	ProjectID          string `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	EnvironmentID      string `json:"environment_id,omitempty" yaml:"environment_id,omitempty"`
	ControllerEndpoint string `json:"controller_endpoint,omitempty" yaml:"controller_endpoint,omitempty"`
	AgentID            string `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
}

// Legacy types.
type Source struct {
	ID, Name, Driver, Path string
	Enabled                bool
}
type Repository struct {
	ID, Name    string
	ChunkBytes  int64
	KeyID       string
	Retention   Retention
	BudgetBytes int64
}
type Retention struct {
	KeepLast, Daily, Weekly, Monthly int
	TombstoneGrace                   string
	GCSweepInterval                  string
}
type Destination struct {
	ID, Driver, Path, Profile, Endpoint, Region, Bucket, Prefix, AccessKeyID, SecretAccessKey, AccessKeyIDFile, SecretAccessKeyFile, AccessKeyIDEnv, SecretAccessKeyEnv string
	UsePathStyle                                                                                                                                                        bool
}
type Keys struct {
	Directory string `json:"directory" yaml:"directory"`
}
type Schedule struct {
	Cron         string `json:"cron,omitempty" yaml:"cron,omitempty"`
	Timezone     string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	Enabled      bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	EverySeconds int    `json:"every_seconds,omitempty" yaml:"every_seconds,omitempty"`
	Workers      int    `json:"workers,omitempty" yaml:"workers,omitempty"`
}

type RuntimeBinding struct {
	Source     SourceConfig
	Repository RepositoryConfig
	Primary    DestinationConfig
	Mirrors    []DestinationConfig
	Replicas   []DestinationConfig
}

func Default() Config {
	enabled := true
	cfg := Config{
		Version:         CurrentVersion,
		Server:          Server{DataDirectory: "./data", ScratchDirectory: "./data/scratch", LogSpoolDirectory: "./data/log-spool", ShutdownGrace: "30s", Listen: "127.0.0.1:8080", API: ListenerConfig{Enabled: true, Listen: "127.0.0.1:8080"}, Metrics: ListenerConfig{Enabled: true, Listen: "127.0.0.1:9090"}},
		Catalogue:       Catalogue{Driver: "sqlite", Path: "./data/dbvault.sqlite", SQLite: CatalogueSQLite{JournalMode: "WAL", Synchronous: "FULL", BusyTimeout: "5s"}},
		SecretProviders: []SecretProviderConfig{{ID: "local-files", Driver: "file", File: &FileSecretProvider{Root: "./data/keys", RejectSymlinks: true, RequireMode: "0600"}}},
		Destinations:    []DestinationConfig{{ID: "filesystem", Driver: "filesystem", Filesystem: &FilesystemDestinationConfig{Root: "./repository", FileMode: "0600", DirectoryMode: "0700"}}},
		Repositories:    []RepositoryConfig{{ID: "repo_local", Mode: "single", Primary: &RepositoryPrimary{Destination: "filesystem"}, Encryption: RepositoryEncryptionConfig{KeyProvider: "local-files", ActiveKey: "local-key"}, Signing: RepositorySigningConfig{KeyProvider: "local-files", KeyID: "signing-local"}, Compression: RepositoryCompressionConfig{Algorithm: "zstd", Level: 6, MinimumSavingsPercent: 5}, Chunking: RepositoryChunkingConfig{SQLite: ChunkStrategyConfig{Strategy: "page-aligned", TargetSize: "4MiB"}, Streams: ChunkStrategyConfig{Strategy: "fixed-stream-v1", TargetSize: "8MiB"}}, Retention: RetentionConfig{KeepLast: 4, Daily: 7, Weekly: 4, Monthly: 3, MinimumVerifiedSnapshots: 2, TombstoneGrace: "168h"}, Budget: BudgetConfig{MaximumPhysicalBytes: "8GiB", ReservePercent: 15}}},
		Sources:         []SourceConfig{{ID: "source_local", Engine: "sqlite", Repository: "repo_local", Enabled: &enabled, SQLite: &SQLiteSourceConfig{Path: "/absolute/path/to/application.sqlite", BusyTimeout: "5m", PagesPerStep: 256}}},
	}
	cfg.syncLegacy()
	return cfg
}

func (c *Config) Normalize() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if c.Server.Listen == "" {
		c.Server.Listen = c.Server.API.Listen
	}
	if c.Server.API.Listen == "" {
		c.Server.API.Listen = c.Server.Listen
	}
	if c.Server.API.Listen == "" {
		c.Server.API.Listen = "127.0.0.1:8080"
	}
	if c.Server.Listen == "" {
		c.Server.Listen = c.Server.API.Listen
	}
	if c.Server.ShutdownGrace == "" {
		c.Server.ShutdownGrace = "30s"
	}
	if c.Server.LogSpoolDirectory == "" && c.Server.DataDirectory != "" {
		c.Server.LogSpoolDirectory = filepath.Join(c.Server.DataDirectory, "log-spool")
	}
	if c.Catalogue.Driver == "" {
		c.Catalogue.Driver = "sqlite"
	}
	if len(c.SecretProviders) == 0 && c.Keys.Directory != "" {
		c.SecretProviders = []SecretProviderConfig{{ID: "local-files", Driver: "file", File: &FileSecretProvider{Root: c.Keys.Directory, RejectSymlinks: true, RequireMode: "0600"}}}
	}
	if len(c.Destinations) == 0 && c.Destination.ID != "" {
		d := DestinationConfig{ID: c.Destination.ID, Driver: c.Destination.Driver, Profile: c.Destination.Profile}
		if d.Driver == "" || d.Driver == "filesystem" {
			d.Driver = "filesystem"
			d.Filesystem = &FilesystemDestinationConfig{Root: c.Destination.Path}
		} else if d.Driver == "s3" {
			d.S3 = &S3DestinationConfig{Endpoint: c.Destination.Endpoint, Region: c.Destination.Region, Bucket: c.Destination.Bucket, Prefix: c.Destination.Prefix, Addressing: S3AddressingConfig{PathStyle: c.Destination.UsePathStyle}, Credentials: S3CredentialConfig{AccessKeyID: legacySecret(c.Destination.AccessKeyID, c.Destination.AccessKeyIDFile, c.Destination.AccessKeyIDEnv), SecretAccessKey: legacySecret(c.Destination.SecretAccessKey, c.Destination.SecretAccessKeyFile, c.Destination.SecretAccessKeyEnv)}}
		}
		c.Destinations = []DestinationConfig{d}
	}
	if len(c.Repositories) == 0 && c.Repository.ID != "" {
		budget := ""
		if c.Repository.BudgetBytes > 0 {
			budget = strconv.FormatInt(c.Repository.BudgetBytes, 10)
		}
		keyProvider := "local-files"
		c.Repositories = []RepositoryConfig{{ID: c.Repository.ID, Mode: "single", Primary: &RepositoryPrimary{Destination: c.Destination.ID}, Encryption: RepositoryEncryptionConfig{KeyProvider: keyProvider, ActiveKey: c.Repository.KeyID}, Signing: RepositorySigningConfig{KeyProvider: keyProvider, KeyID: "signing-local"}, Compression: RepositoryCompressionConfig{Algorithm: "zstd", Level: 6, MinimumSavingsPercent: 5}, Chunking: RepositoryChunkingConfig{SQLite: ChunkStrategyConfig{Strategy: "page-aligned", TargetSize: bytesString(c.Repository.ChunkBytes)}}, Retention: RetentionConfig{KeepLast: c.Repository.Retention.KeepLast, Daily: c.Repository.Retention.Daily, Weekly: c.Repository.Retention.Weekly, Monthly: c.Repository.Retention.Monthly, MinimumVerifiedSnapshots: 2, TombstoneGrace: c.Repository.Retention.TombstoneGrace, GCSweepInterval: c.Repository.Retention.GCSweepInterval}, Budget: BudgetConfig{MaximumPhysicalBytes: budget, ReservePercent: 15}}}
	}
	if len(c.Sources) == 0 && c.Source.ID != "" {
		enabled := c.Source.Enabled
		c.Sources = []SourceConfig{{ID: c.Source.ID, Engine: firstNonEmpty(c.Source.Driver, "sqlite"), Repository: c.Repository.ID, Enabled: &enabled, SQLite: &SQLiteSourceConfig{Path: c.Source.Path, BusyTimeout: "5m", PagesPerStep: 256}}}
	}
	if len(c.Schedules) == 0 && c.Schedule.Enabled {
		enabled := true
		c.Schedules = []ScheduleConfig{{ID: "legacy-schedule", Source: c.Source.ID, Operation: "logical_backup", Cron: c.Schedule.Cron, Timezone: c.Schedule.Timezone, Enabled: &enabled}}
	}
	for i := range c.Destinations {
		applyDestinationDefaults(&c.Destinations[i])
	}
	for i := range c.Repositories {
		applyRepositoryDefaults(&c.Repositories[i])
	}
	for i := range c.Sources {
		applySourceDefaults(&c.Sources[i])
	}
	c.syncLegacy()
}

func legacySecret(literal, file, env string) SecretReference {
	if file != "" {
		return SecretReference{File: file}
	}
	if env != "" {
		return SecretReference{Env: env}
	}
	return SecretReference{Literal: literal}
}
func firstNonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
func bytesString(v int64) string {
	if v <= 0 {
		return "4MiB"
	}
	return strconv.FormatInt(v, 10)
}

func applyDestinationDefaults(d *DestinationConfig) {
	if d.Driver == "" {
		d.Driver = "filesystem"
	}
	if d.S3 != nil {
		switch d.Profile {
		case "cloudflare-r2":
			if d.S3.Region == "" {
				d.S3.Region = "auto"
			}
		case "contabo":
			if d.S3.Region == "" {
				d.S3.Region = "default"
			}
			d.S3.Addressing.PathStyle = true
		case "minio":
			if d.S3.Region == "" {
				d.S3.Region = "us-east-1"
			}
			d.S3.Addressing.PathStyle = true
		}
		if d.S3.Transport.MaximumAttempts == 0 {
			d.S3.Transport.MaximumAttempts = 5
		}
		if d.S3.Transport.RequestTimeout == "" {
			d.S3.Transport.RequestTimeout = "2m"
		}
		if d.S3.Multipart.Threshold == "" {
			d.S3.Multipart.Threshold = "16MiB"
		}
		if d.S3.Multipart.PartSize == "" {
			d.S3.Multipart.PartSize = "16MiB"
		}
		if d.S3.Multipart.Concurrency == 0 {
			d.S3.Multipart.Concurrency = 4
		}
	}
}
func applyRepositoryDefaults(r *RepositoryConfig) {
	if r.Mode == "" {
		r.Mode = "single"
	}
	if r.Compression.Algorithm == "" {
		r.Compression.Algorithm = "zstd"
	}
	if r.Compression.Level == 0 {
		r.Compression.Level = 6
	}
	if r.Compression.MinimumSavingsPercent == 0 {
		r.Compression.MinimumSavingsPercent = 5
	}
	if r.Chunking.SQLite.TargetSize == "" {
		r.Chunking.SQLite = ChunkStrategyConfig{Strategy: "page-aligned", TargetSize: "4MiB"}
	}
	if r.Chunking.Streams.TargetSize == "" {
		r.Chunking.Streams = ChunkStrategyConfig{Strategy: "fixed-stream-v1", TargetSize: "8MiB"}
	}
	if r.Retention.MinimumVerifiedSnapshots == 0 {
		r.Retention.MinimumVerifiedSnapshots = 2
	}
	if r.Retention.TombstoneGrace == "" {
		r.Retention.TombstoneGrace = "168h"
	}
	if r.Budget.ReservePercent == 0 {
		r.Budget.ReservePercent = 15
	}
}
func applySourceDefaults(s *SourceConfig) {
	s.Engine = strings.ToLower(s.Engine)
	if s.Postgres != nil && s.Postgres.Port == 0 {
		s.Postgres.Port = 5432
	}
	if s.MySQL != nil && s.MySQL.Port == 0 {
		s.MySQL.Port = 3306
	}
}

func (c *Config) syncLegacy() {
	if len(c.Sources) > 0 {
		s := c.Sources[0]
		c.Source = Source{ID: s.ID, Name: s.ID, Driver: s.Engine, Enabled: s.IsEnabled()}
		if s.SQLite != nil {
			c.Source.Path = s.SQLite.Path
		}
	}
	if len(c.Repositories) > 0 {
		r := c.Repositories[0]
		chunk, _ := ParseBytes(r.Chunking.SQLite.TargetSize)
		budget, _ := ParseBytes(r.Budget.MaximumPhysicalBytes)
		c.Repository = Repository{ID: r.ID, Name: r.ID, ChunkBytes: chunk, KeyID: r.Encryption.ActiveKey, Retention: Retention{KeepLast: r.Retention.KeepLast, Daily: r.Retention.Daily, Weekly: r.Retention.Weekly, Monthly: r.Retention.Monthly, TombstoneGrace: r.Retention.TombstoneGrace, GCSweepInterval: r.Retention.GCSweepInterval}, BudgetBytes: budget}
	}
	if len(c.Destinations) > 0 {
		d := c.Destinations[0]
		c.Destination = Destination{ID: d.ID, Driver: d.Driver, Profile: d.Profile}
		if d.Filesystem != nil {
			c.Destination.Path = d.Filesystem.Root
		}
		if d.S3 != nil {
			c.Destination.Endpoint, c.Destination.Region, c.Destination.Bucket, c.Destination.Prefix, c.Destination.UsePathStyle = d.S3.Endpoint, d.S3.Region, d.S3.Bucket, d.S3.Prefix, d.S3.Addressing.PathStyle
			applyLegacySecret(&c.Destination.AccessKeyID, &c.Destination.AccessKeyIDFile, &c.Destination.AccessKeyIDEnv, d.S3.Credentials.AccessKeyID)
			applyLegacySecret(&c.Destination.SecretAccessKey, &c.Destination.SecretAccessKeyFile, &c.Destination.SecretAccessKeyEnv, d.S3.Credentials.SecretAccessKey)
		}
	}
	if c.Keys.Directory == "" {
		for _, p := range c.SecretProviders {
			if p.Driver == "file" && p.File != nil {
				c.Keys.Directory = p.File.Root
				break
			}
		}
	}
	if len(c.Schedules) > 0 {
		s := c.Schedules[0]
		c.Schedule = Schedule{Cron: s.Cron, Timezone: s.Timezone, Enabled: s.Enabled == nil || *s.Enabled}
	}
}
func applyLegacySecret(literal, file, env *string, r SecretReference) {
	*literal, *file, *env = r.Literal, r.File, r.Env
}

func Validate(c Config) error {
	c.Normalize()
	var errs []error
	if c.Version < 1 || c.Version > CurrentVersion {
		errs = append(errs, fmt.Errorf("unsupported config version %d", c.Version))
	}
	for name, p := range map[string]string{"server.data_directory": c.Server.DataDirectory, "server.scratch_directory": c.Server.ScratchDirectory, "catalogue.path": c.Catalogue.Path} {
		if strings.TrimSpace(p) == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
	}
	if c.Catalogue.Driver != "" && c.Catalogue.Driver != "sqlite" {
		errs = append(errs, fmt.Errorf("catalogue.driver %q is unsupported", c.Catalogue.Driver))
	}
	if c.Server.ShutdownGrace != "" {
		if _, err := ParseDuration(c.Server.ShutdownGrace); err != nil {
			errs = append(errs, fmt.Errorf("server.shutdown_grace: %w", err))
		}
	}
	providerIDs := map[string]SecretProviderConfig{}
	for _, p := range c.SecretProviders {
		if err := uniqueID("secret provider", p.ID, providerIDs); err != nil {
			errs = append(errs, err)
			continue
		}
		providerIDs[p.ID] = p
		if p.Driver != "file" {
			errs = append(errs, fmt.Errorf("secret provider %s: unsupported driver %q", p.ID, p.Driver))
		}
		if p.Driver == "file" && (p.File == nil || p.File.Root == "") {
			errs = append(errs, fmt.Errorf("secret provider %s: file.root is required", p.ID))
		}
	}
	destinationIDs := map[string]DestinationConfig{}
	for _, d := range c.Destinations {
		if err := uniqueID("destination", d.ID, destinationIDs); err != nil {
			errs = append(errs, err)
			continue
		}
		destinationIDs[d.ID] = d
		errs = append(errs, validateDestination(d, providerIDs)...)
	}
	repositoryIDs := map[string]RepositoryConfig{}
	for _, r := range c.Repositories {
		if err := uniqueID("repository", r.ID, repositoryIDs); err != nil {
			errs = append(errs, err)
			continue
		}
		repositoryIDs[r.ID] = r
		errs = append(errs, validateRepository(r, destinationIDs, providerIDs)...)
	}
	sourceIDs := map[string]SourceConfig{}
	for _, s := range c.Sources {
		if err := uniqueID("source", s.ID, sourceIDs); err != nil {
			errs = append(errs, err)
			continue
		}
		sourceIDs[s.ID] = s
		if _, ok := repositoryIDs[s.Repository]; !ok {
			errs = append(errs, fmt.Errorf("source %s references unknown repository %q", s.ID, s.Repository))
		}
		errs = append(errs, validateSource(s, providerIDs)...)
	}
	for _, s := range c.Schedules {
		if s.ID == "" {
			errs = append(errs, errors.New("schedule id is required"))
		}
		if _, ok := sourceIDs[s.Source]; !ok {
			errs = append(errs, fmt.Errorf("schedule %s references unknown source %q", s.ID, s.Source))
		}
		switch s.Operation {
		case "", "logical_backup", "warehouse_sync":
		default:
			errs = append(errs, fmt.Errorf("schedule %s has unsupported operation %q (supported: logical_backup, warehouse_sync)", s.ID, s.Operation))
		}
		if s.Cron == "" && s.EverySeconds <= 0 {
			errs = append(errs, fmt.Errorf("schedule %s requires cron or every_seconds", s.ID))
		}
	}
	if strings.EqualFold(c.ControlPlane.Mode, "production") {
		for _, reference := range allSecretReferences(c) {
			if reference.Literal != "" {
				errs = append(errs, errors.New("literal secrets are disabled in production mode"))
				break
			}
		}
	}
	if len(c.Sources) == 0 {
		errs = append(errs, errors.New("at least one source is required"))
	}
	if len(c.Repositories) == 0 {
		errs = append(errs, errors.New("at least one repository is required"))
	}
	if len(c.Destinations) == 0 {
		errs = append(errs, errors.New("at least one destination is required"))
	}
	return errors.Join(errs...)
}

func uniqueID[T any](kind, id string, existing map[string]T) error {
	if id == "" {
		return fmt.Errorf("%s id is required", kind)
	}
	if _, ok := existing[id]; ok {
		return fmt.Errorf("duplicate %s id %q", kind, id)
	}
	return nil
}
func validateDestination(d DestinationConfig, providers map[string]SecretProviderConfig) []error {
	var errs []error
	switch d.Driver {
	case "filesystem":
		if d.Filesystem == nil || d.Filesystem.Root == "" {
			errs = append(errs, fmt.Errorf("destination %s filesystem.root is required", d.ID))
		}
	case "s3":
		if d.S3 == nil {
			errs = append(errs, fmt.Errorf("destination %s s3 config is required", d.ID))
			break
		}
		if d.S3.Endpoint == "" || d.S3.Bucket == "" {
			errs = append(errs, fmt.Errorf("destination %s s3 endpoint and bucket are required", d.ID))
		}
		if u, err := url.Parse(d.S3.Endpoint); err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" {
			errs = append(errs, fmt.Errorf("destination %s has invalid s3 endpoint", d.ID))
		} else if u.Scheme != "https" && !d.S3.TLS.AllowInsecureHTTP {
			errs = append(errs, fmt.Errorf("destination %s requires HTTPS unless allow_insecure_http is true", d.ID))
		}
		if err := validatePrefix(d.S3.Prefix); err != nil {
			errs = append(errs, fmt.Errorf("destination %s prefix: %w", d.ID, err))
		}
		errs = append(errs, validateSecretRef("destination "+d.ID+" access key", d.S3.Credentials.AccessKeyID, providers, true)...)
		errs = append(errs, validateSecretRef("destination "+d.ID+" secret key", d.S3.Credentials.SecretAccessKey, providers, true)...)
	default:
		errs = append(errs, fmt.Errorf("destination %s uses unsupported driver %q", d.ID, d.Driver))
	}
	return errs
}
func validateRepository(r RepositoryConfig, destinations map[string]DestinationConfig, providers map[string]SecretProviderConfig) []error {
	var errs []error
	refs := []string{}
	if r.Primary != nil {
		refs = append(refs, r.Primary.Destination)
	}
	for _, m := range r.Mirrors {
		refs = append(refs, m.Destination)
	}
	for _, rp := range r.Replicas {
		refs = append(refs, rp.Destination)
		if rp.RequiredWithin != "" {
			if _, err := ParseDuration(rp.RequiredWithin); err != nil {
				errs = append(errs, fmt.Errorf("repository %s replica %s required_within: %w", r.ID, rp.Destination, err))
			}
		}
	}
	if len(refs) == 0 {
		errs = append(errs, fmt.Errorf("repository %s has no destinations", r.ID))
	}
	for _, id := range refs {
		if _, ok := destinations[id]; !ok {
			errs = append(errs, fmt.Errorf("repository %s references unknown destination %q", r.ID, id))
		}
	}
	switch r.Mode {
	case "single", "mirror", "primary_replica":
	default:
		errs = append(errs, fmt.Errorf("repository %s has invalid mode %q", r.ID, r.Mode))
	}
	if r.Mode == "single" || r.Mode == "primary_replica" {
		if r.Primary == nil || r.Primary.Destination == "" {
			errs = append(errs, fmt.Errorf("repository %s requires primary.destination", r.ID))
		}
	}
	if r.Mode == "mirror" && len(r.Mirrors) < 2 {
		errs = append(errs, fmt.Errorf("repository %s mirror mode requires at least two mirrors", r.ID))
	}
	if r.Encryption.ActiveKey == "" {
		errs = append(errs, fmt.Errorf("repository %s encryption.active_key is required", r.ID))
	}
	if r.Encryption.KeyProvider != "" {
		if _, ok := providers[r.Encryption.KeyProvider]; !ok {
			errs = append(errs, fmt.Errorf("repository %s references unknown encryption key provider %q", r.ID, r.Encryption.KeyProvider))
		}
	}
	if r.Signing.KeyProvider != "" {
		if _, ok := providers[r.Signing.KeyProvider]; !ok {
			errs = append(errs, fmt.Errorf("repository %s references unknown signing key provider %q", r.ID, r.Signing.KeyProvider))
		}
	}
	if r.Retention.MinimumVerifiedSnapshots < 2 {
		errs = append(errs, fmt.Errorf("repository %s minimum_verified_snapshots must be at least 2", r.ID))
	}
	if r.Retention.TombstoneGrace != "" {
		if _, err := ParseDuration(r.Retention.TombstoneGrace); err != nil {
			errs = append(errs, fmt.Errorf("repository %s tombstone_grace: %w", r.ID, err))
		}
	}
	if r.Budget.MaximumPhysicalBytes != "" {
		if _, err := ParseBytes(r.Budget.MaximumPhysicalBytes); err != nil {
			errs = append(errs, fmt.Errorf("repository %s maximum_physical_bytes: %w", r.ID, err))
		}
	}
	return errs
}
func validateSource(s SourceConfig, providers map[string]SecretProviderConfig) []error {
	var errs []error
	switch s.Engine {
	case "sqlite":
		if s.SQLite == nil || s.SQLite.Path == "" {
			errs = append(errs, fmt.Errorf("source %s sqlite.path is required", s.ID))
		} else if !filepath.IsAbs(s.SQLite.Path) {
			errs = append(errs, fmt.Errorf("source %s sqlite.path must be absolute", s.ID))
		}
	case "postgres":
		if s.Postgres == nil {
			errs = append(errs, fmt.Errorf("source %s postgres config is required", s.ID))
			break
		}
		if s.Postgres.Host == "" || s.Postgres.Database == "" || s.Postgres.Username == "" {
			errs = append(errs, fmt.Errorf("source %s postgres host, database and username are required", s.ID))
		}
		errs = append(errs, validateSecretRef("source "+s.ID+" postgres password", s.Postgres.Password, providers, true)...)
	case "mysql", "mariadb":
		if s.MySQL == nil {
			errs = append(errs, fmt.Errorf("source %s mysql config is required", s.ID))
			break
		}
		if s.MySQL.Host == "" || s.MySQL.Database == "" || s.MySQL.Username == "" {
			errs = append(errs, fmt.Errorf("source %s mysql host, database and username are required", s.ID))
		}
		errs = append(errs, validateSecretRef("source "+s.ID+" mysql password", s.MySQL.Password, providers, true)...)
	default:
		errs = append(errs, fmt.Errorf("source %s uses unsupported engine %q", s.ID, s.Engine))
	}
	return errs
}
func validateSecretRef(name string, r SecretReference, providers map[string]SecretProviderConfig, required bool) []error {
	if r.Empty() {
		if required {
			return []error{fmt.Errorf("%s is required", name)}
		}
		return nil
	}
	set := 0
	if r.Provider != "" || r.Key != "" {
		set++
		if r.Provider == "" || r.Key == "" {
			return []error{fmt.Errorf("%s provider and key must be specified together", name)}
		}
		if _, ok := providers[r.Provider]; !ok {
			return []error{fmt.Errorf("%s references unknown provider %q", name, r.Provider)}
		}
	}
	if r.Env != "" {
		set++
	}
	if r.File != "" {
		set++
	}
	if r.Literal != "" {
		set++
	}
	if set != 1 {
		return []error{fmt.Errorf("%s must use exactly one secret source", name)}
	}
	return nil
}
func validatePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "\\") || strings.Contains(prefix, "..") {
		return errors.New("unsafe path")
	}
	for _, part := range strings.Split(prefix, "/") {
		if part == "" {
			return errors.New("empty path component")
		}
	}
	return nil
}

func (c Config) Binding(sourceID string) (RuntimeBinding, error) {
	c.Normalize()
	var source SourceConfig
	found := false
	for _, s := range c.Sources {
		if (sourceID == "" && s.IsEnabled()) || s.ID == sourceID {
			source, found = s, true
			break
		}
	}
	if !found {
		return RuntimeBinding{}, fmt.Errorf("source %q not found", sourceID)
	}
	var repo RepositoryConfig
	for _, r := range c.Repositories {
		if r.ID == source.Repository {
			repo = r
			found = true
			break
		}
	}
	if repo.ID == "" {
		return RuntimeBinding{}, fmt.Errorf("repository %q not found", source.Repository)
	}
	lookup := func(id string) (DestinationConfig, error) {
		for _, d := range c.Destinations {
			if d.ID == id {
				return d, nil
			}
		}
		return DestinationConfig{}, fmt.Errorf("destination %q not found", id)
	}
	var primary DestinationConfig
	if repo.Primary != nil {
		var err error
		primary, err = lookup(repo.Primary.Destination)
		if err != nil {
			return RuntimeBinding{}, err
		}
	}
	mirrors := make([]DestinationConfig, 0, len(repo.Mirrors))
	for _, m := range repo.Mirrors {
		d, err := lookup(m.Destination)
		if err != nil {
			return RuntimeBinding{}, err
		}
		mirrors = append(mirrors, d)
	}
	replicas := make([]DestinationConfig, 0, len(repo.Replicas))
	for _, rp := range repo.Replicas {
		d, err := lookup(rp.Destination)
		if err != nil {
			return RuntimeBinding{}, err
		}
		replicas = append(replicas, d)
	}
	if primary.ID == "" && len(mirrors) > 0 {
		primary = mirrors[0]
	}
	return RuntimeBinding{Source: source, Repository: repo, Primary: primary, Mirrors: mirrors, Replicas: replicas}, nil
}

func (c Config) ResolveSecret(r SecretReference, production bool) (string, error) {
	c.Normalize()
	if r.Literal != "" {
		if production {
			return "", errors.New("literal secrets are disabled in production")
		}
		return r.Literal, nil
	}
	if r.Env != "" {
		v, ok := os.LookupEnv(r.Env)
		if !ok {
			return "", fmt.Errorf("environment secret %s is not set", r.Env)
		}
		return strings.TrimSpace(v), nil
	}
	path := r.File
	if r.Provider != "" {
		var provider *SecretProviderConfig
		for i := range c.SecretProviders {
			if c.SecretProviders[i].ID == r.Provider {
				provider = &c.SecretProviders[i]
				break
			}
		}
		if provider == nil || provider.File == nil {
			return "", fmt.Errorf("secret provider %q is unavailable", r.Provider)
		}
		if filepath.IsAbs(r.Key) || strings.Contains(r.Key, "..") {
			return "", errors.New("unsafe secret key")
		}
		path = filepath.Join(provider.File.Root, filepath.FromSlash(r.Key))
	}
	if path == "" {
		return "", errors.New("empty secret reference")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("secret symlinks are not allowed")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (c Config) Redacted() Config {
	out := c
	redact := func(r *SecretReference) {
		if !r.Empty() {
			r.Literal = "<redacted>"
		}
	}
	for i := range out.Destinations {
		if out.Destinations[i].S3 != nil {
			redact(&out.Destinations[i].S3.Credentials.AccessKeyID)
			redact(&out.Destinations[i].S3.Credentials.SecretAccessKey)
			redact(&out.Destinations[i].S3.Credentials.SessionToken)
		}
	}
	for i := range out.Sources {
		if out.Sources[i].Postgres != nil {
			redact(&out.Sources[i].Postgres.Password)
		}
		if out.Sources[i].MySQL != nil {
			redact(&out.Sources[i].MySQL.Password)
		}
	}
	redact(&out.Notifications.Webhook.URL)
	redact(&out.Notifications.Webhook.SigningSecret)
	out.Destination.AccessKeyID, out.Destination.SecretAccessKey = "<redacted>", "<redacted>"
	return out
}

func (c Config) ExplainRepository(id string) (string, error) {
	c.Normalize()
	var repo RepositoryConfig
	for _, r := range c.Repositories {
		if r.ID == id {
			repo = r
			break
		}
	}
	if repo.ID == "" {
		return "", fmt.Errorf("repository %q not found", id)
	}
	lines := []string{"Repository: " + repo.ID, "Mode: " + repo.Mode}
	if repo.Primary != nil {
		lines = append(lines, "Primary: "+repo.Primary.Destination)
	}
	for _, m := range repo.Mirrors {
		lines = append(lines, fmt.Sprintf("Mirror: %s (required=%t)", m.Destination, m.Required))
	}
	for _, r := range repo.Replicas {
		lines = append(lines, "Replica: "+r.Destination+" required within "+r.RequiredWithin)
	}
	var sources []string
	for _, s := range c.Sources {
		if s.Repository == id {
			sources = append(sources, s.ID)
		}
	}
	sort.Strings(sources)
	lines = append(lines, "Sources: "+strings.Join(sources, ", "))
	lines = append(lines, "Encryption key: "+repo.Encryption.ActiveKey)
	lines = append(lines, fmt.Sprintf("Retention: %d daily, %d weekly, %d monthly", repo.Retention.Daily, repo.Retention.Weekly, repo.Retention.Monthly))
	lines = append(lines, "Budget: "+repo.Budget.MaximumPhysicalBytes)
	return strings.Join(lines, "\n"), nil
}

func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	lower := strings.ToLower(s)
	for suffix, m := range map[string]int64{"tib": 1 << 40, "tb": 1e12, "gib": 1 << 30, "gb": 1e9, "mib": 1 << 20, "mb": 1e6, "kib": 1 << 10, "kb": 1e3} {
		if strings.HasSuffix(lower, suffix) {
			mult = m
			s = strings.TrimSpace(s[:len(s)-len(suffix)])
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(f * float64(mult)), nil
}

func MarshalRedactedJSON(c Config) ([]byte, error) { return json.MarshalIndent(c.Redacted(), "", "  ") }

func allSecretReferences(c Config) []SecretReference {
	out := []SecretReference{c.Notifications.Webhook.URL, c.Notifications.Webhook.SigningSecret}
	for _, destination := range c.Destinations {
		if destination.S3 != nil {
			out = append(out, destination.S3.Credentials.AccessKeyID, destination.S3.Credentials.SecretAccessKey, destination.S3.Credentials.SessionToken)
		}
	}
	for _, source := range c.Sources {
		if source.Postgres != nil {
			out = append(out, source.Postgres.Password)
		}
		if source.MySQL != nil {
			out = append(out, source.MySQL.Password)
		}
	}
	return out
}

func ParseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	lower := strings.ToLower(value)
	for suffix, multiplier := range map[string]time.Duration{"w": 7 * 24 * time.Hour, "d": 24 * time.Hour} {
		if strings.HasSuffix(lower, suffix) {
			number := strings.TrimSpace(value[:len(value)-len(suffix)])
			parsed, err := strconv.ParseFloat(number, 64)
			if err != nil {
				return 0, err
			}
			return time.Duration(parsed * float64(multiplier)), nil
		}
	}
	return time.ParseDuration(value)
}
