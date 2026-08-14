package domain

import "time"

type RetentionPolicy struct {
	KeepLast                 int
	Daily                    int
	Weekly                   int
	Monthly                  int
	MinimumVerifiedSnapshots int
	KeepLatestRestoreTested  bool
	TombstoneGracePeriod     time.Duration
	ChunkGCGracePeriod       time.Duration
}

type BudgetLimitAction string

const (
	BudgetFailBackup    BudgetLimitAction = "fail_backup"
	BudgetPauseSchedule BudgetLimitAction = "pause_schedule"
	BudgetAlertOnly     BudgetLimitAction = "alert_only"
)

type BudgetPolicy struct {
	MaximumRepositoryBytes int64
	WarningPercent         int
	CriticalPercent        int
	ReservePercent         int
	MinimumReserveBytes    int64
	OnLimit                BudgetLimitAction
}

type RetentionClass string

const (
	RetentionLatest      RetentionClass = "latest"
	RetentionRecent      RetentionClass = "recent"
	RetentionDaily       RetentionClass = "daily"
	RetentionWeekly      RetentionClass = "weekly"
	RetentionMonthly     RetentionClass = "monthly"
	RetentionProtected   RetentionClass = "protected"
	RetentionRestoreTest RetentionClass = "restore_tested"
)

type RetentionDecision struct {
	Keep      map[SnapshotID][]RetentionClass
	Tombstone []SnapshotID
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		KeepLast: 4, Daily: 7, Weekly: 4, Monthly: 3,
		MinimumVerifiedSnapshots: 2, KeepLatestRestoreTested: true,
		TombstoneGracePeriod: 7 * 24 * time.Hour,
		ChunkGCGracePeriod:   7 * 24 * time.Hour,
	}
}
