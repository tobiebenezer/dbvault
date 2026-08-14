package domain

import "time"

type PhysicalBackupID string

type PhysicalBackupType string

const (
	PhysicalBackupFull        PhysicalBackupType = "full"
	PhysicalBackupIncremental PhysicalBackupType = "incremental"
)

type LogPosition struct {
	Engine    DatabaseEngine `json:"engine"`
	Timeline  string         `json:"timeline,omitempty"`
	LSN       string         `json:"lsn,omitempty"`
	File      string         `json:"file,omitempty"`
	Position  uint64         `json:"position,omitempty"`
	GTIDSet   string         `json:"gtid_set,omitempty"`
	Timestamp *time.Time     `json:"timestamp,omitempty"`
}

type PhysicalBackupSet struct {
	ID               PhysicalBackupID  `json:"id"`
	SourceID         SourceID          `json:"source_id"`
	RepositoryID     RepositoryID      `json:"repository_id"`
	LineageID        LineageID         `json:"lineage_id"`
	Engine           DatabaseEngine    `json:"engine"`
	EngineVersion    string            `json:"engine_version"`
	Type             PhysicalBackupType `json:"type"`
	ParentBackupID   *PhysicalBackupID `json:"parent_backup_id,omitempty"`
	StartedAt        time.Time         `json:"started_at"`
	CompletedAt      time.Time         `json:"completed_at"`
	StartLogPosition LogPosition       `json:"start_log_position"`
	EndLogPosition   LogPosition       `json:"end_log_position"`
	Timeline         string            `json:"timeline,omitempty"`
	SystemIdentifier string            `json:"system_identifier,omitempty"`
	Artifacts        []BackupArtifact  `json:"artifacts"`
	ManifestDigest   string            `json:"manifest_digest"`
	VerifiedAt       *time.Time        `json:"verified_at,omitempty"`
	RestoreTestedAt  *time.Time        `json:"restore_tested_at,omitempty"`
}

func (b PhysicalBackupSet) IsIncremental() bool { return b.Type == PhysicalBackupIncremental }

func (b PhysicalBackupSet) ValidateChainParent(parent PhysicalBackupSet) error {
	if b.Type != PhysicalBackupIncremental {
		return nil
	}
	if b.ParentBackupID == nil || *b.ParentBackupID != parent.ID {
		return NewError(ErrRecoveryChainIncomplete, "incremental backup parent is missing", nil)
	}
	if b.SystemIdentifier != parent.SystemIdentifier || b.LineageID != parent.LineageID {
		return NewError(ErrSystemIdentifierMismatch, "incremental parent belongs to a different lineage", nil)
	}
	return nil
}
