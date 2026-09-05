package domain

import "time"

type TransactionLogID string

type TransactionLogKind string

const (
	LogPostgresWAL   TransactionLogKind = "postgres_wal"
	LogMySQLBinlog   TransactionLogKind = "mysql_binlog"
	LogMariaDBBinlog TransactionLogKind = "mariadb_binlog"
)

type TransactionLogStatus string

const (
	LogCollecting TransactionLogStatus = "collecting"
	LogStored     TransactionLogStatus = "stored"
	LogVerified   TransactionLogStatus = "verified"
	LogAvailable  TransactionLogStatus = "available"
	LogGap        TransactionLogStatus = "gap"
	LogCorrupted  TransactionLogStatus = "corrupted"
	LogExpired    TransactionLogStatus = "expired"
	LogDeleted    TransactionLogStatus = "deleted"
)

type TransactionLog struct {
	ID            TransactionLogID     `json:"id"`
	SourceID      SourceID             `json:"source_id"`
	RepositoryID  RepositoryID         `json:"repository_id"`
	LineageID     LineageID            `json:"lineage_id"`
	Kind          TransactionLogKind   `json:"kind"`
	NativeName    string               `json:"native_name"`
	StartPosition LogPosition          `json:"start_position"`
	EndPosition   LogPosition          `json:"end_position"`
	StartTime     *time.Time           `json:"start_time,omitempty"`
	EndTime       *time.Time           `json:"end_time,omitempty"`
	Timeline      string               `json:"timeline,omitempty"`
	GTIDSet       string               `json:"gtid_set,omitempty"`
	PreviousLogID *TransactionLogID    `json:"previous_log_id,omitempty"`
	NextLogID     *TransactionLogID    `json:"next_log_id,omitempty"`
	LogicalSize   int64                `json:"logical_size"`
	StoredSize    int64                `json:"stored_size"`
	Checksum      string               `json:"checksum"`
	ObjectKey     string               `json:"object_key"`
	KeyID         string               `json:"key_id"`
	Status        TransactionLogStatus `json:"status"`
	CollectedAt   time.Time            `json:"collected_at"`
	VerifiedAt    *time.Time           `json:"verified_at,omitempty"`
}

func (l TransactionLog) IsVerified() bool { return l.Status == LogVerified && l.VerifiedAt != nil }
