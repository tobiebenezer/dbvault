package domain

import "time"

type TimelineHistory struct {
	SourceID         SourceID   `json:"source_id"`
	SystemIdentifier string     `json:"system_identifier"`
	Timeline         uint32     `json:"timeline"`
	ParentTimeline   uint32     `json:"parent_timeline"`
	SwitchLSN        string     `json:"switch_lsn"`
	Reason           string     `json:"reason"`
	ObjectKey        string     `json:"object_key"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
}

type RecoveryPointID string

type NamedRecoveryPoint struct {
	ID        RecoveryPointID `json:"id"`
	SourceID  SourceID        `json:"source_id"`
	Name      string          `json:"name"`
	LSN       string          `json:"lsn,omitempty"`
	Timeline  uint32          `json:"timeline"`
	GTIDSet   string          `json:"gtid_set,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	CreatedBy string          `json:"created_by"`
}

type NamedRestorePoint struct {
	Name      string    `json:"name"`
	LSN       string    `json:"lsn,omitempty"`
	Timeline  uint32    `json:"timeline,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type TimelineGapEvent struct {
	SourceID    SourceID  `json:"source_id"`
	ExpectedLSN string    `json:"expected_lsn"`
	ReceivedLSN string    `json:"received_lsn"`
	DetectedAt  time.Time `json:"detected_at"`
	Severity    string    `json:"severity"`
}
