package domain

import "time"

type KeyStatus string

const (
	KeyActive   KeyStatus = "active"
	KeyRetiring KeyStatus = "retiring"
	KeyRetired  KeyStatus = "retired"
	KeyRevoked  KeyStatus = "revoked"
)

type KeyUsage struct {
	RepositoryID      RepositoryID
	KeyID             string
	Status            KeyStatus
	RetainedChunks    int
	RetainedSnapshots int
	OldestReference   *time.Time
	NewestReference   *time.Time
	SafeToRetire      bool
	BlockingReasons   []string
}
