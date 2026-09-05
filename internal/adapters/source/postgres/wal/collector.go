package wal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// walNameRE matches:
// 1. Standard 24-hex WAL segments and .partial segments (e.g. 000000010000000000000001, 000000010000000000000001.partial)
// 2. Timeline history files (e.g. 00000002.history)
// 3. Base backup label files (e.g. 000000010000000000000001.00000028.backup)
var walNameRE = regexp.MustCompile(`^[0-9A-Fa-f]{24}(?:\.partial)?$|^[0-9A-Fa-f]{8}\.history$|^[0-9A-Fa-f]{24}\.[0-9A-Fa-f]{8}\.backup$`)

type ArchiveHelper struct {
	SourceID     domain.SourceID
	RepositoryID domain.RepositoryID
	Store        ports.ObjectStore
	Now          func() time.Time
}

func ValidName(name string) bool {
	return walNameRE.MatchString(name)
}

func (h ArchiveHelper) Push(ctx context.Context, filePath, name string) (domain.TransactionLog, error) {
	if !ValidName(name) {
		return domain.TransactionLog{}, domain.NewError(domain.ErrWALSegmentInvalid, "invalid WAL filename", nil)
	}
	clean := filepath.Clean(filePath)
	f, err := os.Open(clean)
	if err != nil {
		return domain.TransactionLog{}, err
	}
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, f)
	if err != nil {
		return domain.TransactionLog{}, err
	}
	_, _ = f.Seek(0, 0)
	key := string(h.SourceID) + "/wal/" + name
	if h.Store != nil {
		_, err = h.Store.Put(ctx, ports.PutObjectRequest{
			Key:         key,
			Body:        f,
			Size:        n,
			ContentType: "application/octet-stream",
			IfNotExists: true,
		})
		if err != nil {
			return domain.TransactionLog{}, err
		}
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	return domain.TransactionLog{
		ID:           domain.TransactionLogID(name),
		SourceID:     h.SourceID,
		RepositoryID: h.RepositoryID,
		Kind:         domain.LogPostgresWAL,
		NativeName:   name,
		LogicalSize:  n,
		StoredSize:   n,
		Checksum:     hex.EncodeToString(hash.Sum(nil)),
		ObjectKey:    key,
		Status:       domain.LogVerified,
		CollectedAt:  now,
		VerifiedAt:   &now,
	}, nil
}

type Checkpoint struct {
	SourceID     domain.SourceID
	SystemID     string
	Timeline     uint32
	ConfirmedLSN string
	ReceivedLSN  string
	SlotName     string
	UpdatedAt    time.Time
}
