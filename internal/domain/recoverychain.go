package domain

import "time"

type LineageID string

type LineageStatus string

const (
	LineageActive LineageStatus = "active"
	LineageClosed LineageStatus = "closed"
	LineageBroken LineageStatus = "broken"
)

type RecoveryLineage struct {
	ID             LineageID      `json:"id"`
	SourceID       SourceID       `json:"source_id"`
	Engine         DatabaseEngine `json:"engine"`
	NativeIdentity string         `json:"native_identity"`
	StartedAt      time.Time      `json:"started_at"`
	EndedAt        *time.Time     `json:"ended_at,omitempty"`
	Status         LineageStatus  `json:"status"`
}

type RecoveryWindow struct {
	SourceID            SourceID                                  `json:"source_id"`
	LineageID           LineageID                                 `json:"lineage_id"`
	EarliestTime        time.Time                                 `json:"earliest_time"`
	LatestTime          time.Time                                 `json:"latest_time"`
	EarliestPosition    LogPosition                               `json:"earliest_position"`
	LatestPosition      LogPosition                               `json:"latest_position"`
	BaseBackupID        string                                    `json:"base_backup_id"`
	Continuous          bool                                      `json:"continuous"`
	GapCount            int                                       `json:"gap_count"`
	LastVerifiedAt      time.Time                                 `json:"last_verified_at"`
	DestinationCoverage map[DestinationID]DestinationRecoveryCoverage `json:"destination_coverage"`
}

type DestinationRecoveryCoverage struct {
	DestinationID        DestinationID `json:"destination_id"`
	EarliestRecoverable  time.Time     `json:"earliest_recoverable"`
	LatestRecoverable    time.Time     `json:"latest_recoverable"`
	Continuous           bool          `json:"continuous"`
	MissingLogs          int           `json:"missing_logs"`
	LastVerifiedAt       time.Time     `json:"last_verified_at"`
}

func ContinuousWindow(source SourceID, lineage LineageID, base PhysicalBackupSet, logs []TransactionLog, at time.Time) RecoveryWindow {
	w := RecoveryWindow{SourceID: source, LineageID: lineage, BaseBackupID: string(base.ID), EarliestTime: base.StartedAt, LatestTime: base.CompletedAt, Continuous: true, LastVerifiedAt: at}
	if len(logs) > 0 {
		if logs[0].StartTime != nil { w.EarliestTime = *logs[0].StartTime }
		last := logs[len(logs)-1]
		if last.EndTime != nil { w.LatestTime = *last.EndTime }
		w.LatestPosition = last.EndPosition
	}
	return w
}
