package kubernetes

import (
	"errors"
	"fmt"
)

type ObjectMeta struct {
	Name      string            `json:"name" yaml:"name"`
	Namespace string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}
type ObjectReference struct {
	Name      string `json:"name" yaml:"name"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}
type SecretKeyReference struct {
	Name string `json:"name" yaml:"name"`
	Key  string `json:"key" yaml:"key"`
}

type DBVaultDestination struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   ObjectMeta      `json:"metadata"`
	Spec       DestinationSpec `json:"spec"`
}
type DestinationSpec struct {
	Driver, Profile, Endpoint, Region, Bucket, Prefix string
	CredentialsSecretRef                              SecretKeyReference
}

type DBVaultRepository struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   ObjectMeta     `json:"metadata"`
	Spec       RepositorySpec `json:"spec"`
}
type RepositorySpec struct {
	Mode                string
	Primary             ObjectReference
	Replicas            []RepositoryReplica
	EncryptionSecretRef SecretKeyReference
	Retention           RepositoryRetention
}
type RepositoryReplica struct {
	DestinationRef ObjectReference
	RequiredWithin string
}
type RepositoryRetention struct{ KeepLast, Daily, Weekly, Monthly int }

type DBVaultSource struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       SourceSpec `json:"spec"`
}
type SourceSpec struct {
	Engine         string
	RepositoryRef  ObjectReference
	Connection     DatabaseConnection
	LogicalBackup  BackupToggle
	PhysicalBackup BackupToggle
	WAL            BackupToggle
	Binlog         BackupToggle
}
type DatabaseConnection struct {
	Host              string
	Port              int
	Database          string
	Username          string
	PasswordSecretRef SecretKeyReference
	SQLitePath        string
	SSLMode           string
}
type BackupToggle struct {
	Enabled bool
	Mode    string
	Format  string
}

type DBVaultSchedule struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   ObjectMeta   `json:"metadata"`
	Spec       ScheduleSpec `json:"spec"`
}
type ScheduleSpec struct {
	SourceRef                                        ObjectReference
	Operation, Schedule, Timezone, ConcurrencyPolicy string
}

type DBVaultRestore struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   ObjectMeta  `json:"metadata"`
	Spec       RestoreSpec `json:"spec"`
}
type RestoreSpec struct {
	SourceRef        ObjectReference
	SnapshotID       string
	TargetTime       string
	RestorePoint     string
	Target           DatabaseConnection
	ApprovalRequired bool
}

func ValidateSource(source DBVaultSource) error {
	if source.Metadata.Name == "" || source.Spec.RepositoryRef.Name == "" {
		return errors.New("source name and repositoryRef are required")
	}
	switch source.Spec.Engine {
	case "sqlite":
		if source.Spec.Connection.SQLitePath == "" {
			return errors.New("sqlitePath is required")
		}
	case "postgres", "mysql", "mariadb":
		if source.Spec.Connection.Host == "" || source.Spec.Connection.Database == "" || source.Spec.Connection.Username == "" || source.Spec.Connection.PasswordSecretRef.Name == "" {
			return errors.New("network database connection and password secret are required")
		}
	default:
		return fmt.Errorf("unsupported engine %q", source.Spec.Engine)
	}
	return nil
}

func ValidateRepository(repository DBVaultRepository) error {
	if repository.Metadata.Name == "" {
		return errors.New("repository name is required")
	}
	switch repository.Spec.Mode {
	case "single", "primaryReplica":
		if repository.Spec.Primary.Name == "" {
			return errors.New("primary destination is required")
		}
	case "mirror":
		if len(repository.Spec.Replicas) < 2 {
			return errors.New("mirror mode requires at least two destinations")
		}
	default:
		return errors.New("invalid repository mode")
	}
	if repository.Spec.EncryptionSecretRef.Name == "" {
		return errors.New("encryption secret is required")
	}
	return nil
}
