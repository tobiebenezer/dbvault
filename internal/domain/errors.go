package domain

import "fmt"

type ErrorCode string

const (
	ErrConfigurationInvalid      ErrorCode = "configuration_invalid"
	ErrSourceNotFound            ErrorCode = "source_not_found"
	ErrSourceNotSQLite           ErrorCode = "source_not_sqlite"
	ErrSourceBusy                ErrorCode = "source_busy"
	ErrScratchInsufficient       ErrorCode = "scratch_insufficient"
	ErrSnapshotFailed            ErrorCode = "snapshot_failed"
	ErrChunkReadFailed           ErrorCode = "chunk_read_failed"
	ErrQuickCheckFailed          ErrorCode = "quick_check_failed"
	ErrStorageBudgetExceeded     ErrorCode = "storage_budget_exceeded"
	ErrStorageUnavailable        ErrorCode = "storage_unavailable"
	ErrManifestInvalid           ErrorCode = "manifest_invalid"
	ErrChunkMissing              ErrorCode = "chunk_missing"
	ErrChunkAuthenticationFailed ErrorCode = "chunk_authentication_failed"
	ErrStorageAuthentication     ErrorCode = "storage_authentication_failed"
	ErrStoragePermission         ErrorCode = "storage_permission_denied"
	ErrObjectAlreadyExists       ErrorCode = "object_already_exists"
	ErrJobUnavailable            ErrorCode = "job_unavailable"
	ErrVerificationFailed        ErrorCode = "verification_failed"
	ErrRetentionBlocked          ErrorCode = "retention_blocked"
	ErrGCPlanStale               ErrorCode = "gc_plan_stale"
	ErrKeyRetirementBlocked      ErrorCode = "key_retirement_blocked"
)

type AppError struct {
	Code      ErrorCode
	Message   string
	Retryable bool
	Cause     error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(code ErrorCode, msg string, cause error) *AppError {
	return &AppError{Code: code, Message: msg, Cause: cause}
}

const (
	ErrEngineUnsupported        ErrorCode = "engine_unsupported"
	ErrToolMissing              ErrorCode = "database_tool_missing"
	ErrToolVersionUnsupported   ErrorCode = "database_tool_version_unsupported"
	ErrServerVersionUnsupported ErrorCode = "server_version_unsupported"
	ErrDatabaseAuthentication   ErrorCode = "database_authentication_failed"
	ErrDatabasePermission       ErrorCode = "database_permission_denied"
	ErrDatabaseConnection       ErrorCode = "database_connection_failed"
	ErrDumpFailed               ErrorCode = "database_dump_failed"
	ErrArchiveInvalid           ErrorCode = "database_archive_invalid"
	ErrRestoreToolFailed        ErrorCode = "database_restore_failed"
	ErrTargetDatabaseExists     ErrorCode = "target_database_exists"
	ErrTargetDatabaseMissing    ErrorCode = "target_database_missing"
	ErrExtensionMissing         ErrorCode = "database_extension_missing"
	ErrRoleMissing              ErrorCode = "database_role_missing"
	ErrNonTransactionalTables   ErrorCode = "non_transactional_tables"
	ErrHookFailed               ErrorCode = "hook_failed"
)

const (
	ErrPhysicalBackupUnsupported ErrorCode = "physical_backup_unsupported"
	ErrPhysicalBackupFailed      ErrorCode = "physical_backup_failed"
	ErrBackupManifestInvalid     ErrorCode = "backup_manifest_invalid"
	ErrWALArchivingDisabled      ErrorCode = "wal_archiving_disabled"
	ErrWALSegmentInvalid         ErrorCode = "wal_segment_invalid"
	ErrWALGap                    ErrorCode = "wal_gap"
	ErrTimelineInvalid           ErrorCode = "timeline_invalid"
	ErrSystemIdentifierMismatch  ErrorCode = "system_identifier_mismatch"
	ErrReplicationSlotMissing    ErrorCode = "replication_slot_missing"
	ErrReplicationSlotLag        ErrorCode = "replication_slot_lag"
	ErrBinlogDisabled            ErrorCode = "binlog_disabled"
	ErrBinlogInvalid             ErrorCode = "binlog_invalid"
	ErrBinlogGap                 ErrorCode = "binlog_gap"
	ErrGTIDMismatch              ErrorCode = "gtid_mismatch"
	ErrRecoveryTargetInvalid     ErrorCode = "recovery_target_invalid"
	ErrRecoveryWindowUnavailable ErrorCode = "recovery_window_unavailable"
	ErrRecoveryChainIncomplete   ErrorCode = "recovery_chain_incomplete"
	ErrPITRFailed                ErrorCode = "point_in_time_recovery_failed"
	ErrLineageChanged            ErrorCode = "recovery_lineage_changed"
)
