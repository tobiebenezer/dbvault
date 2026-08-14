package repository

import (
	"context"
	"strings"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Mode string

const (
	Quick    Mode = "quick"
	Standard Mode = "standard"
	Deep     Mode = "deep"
	Forensic Mode = "forensic"
)

type SnapshotIssue struct {
	SnapshotID domain.SnapshotID
	Key        string
	Reason     string
}
type ChunkIssue struct {
	ChunkID domain.ChunkID
	Key     string
	Reason  string
}
type DestinationDifference struct {
	DestinationID domain.DestinationID
	Key           string
	Reason        string
}

type ScanResult struct {
	RepositoryID             domain.RepositoryID
	Mode                     Mode
	ValidSnapshots           int
	CorruptedSnapshots       []SnapshotIssue
	MissingChunks            []ChunkIssue
	OrphanChunks             []ChunkIssue
	InvalidSignatures        []SnapshotIssue
	InvalidCompletionMarkers []SnapshotIssue
	RemoteOnlySnapshots      []domain.SnapshotID
	LocalOnlySnapshots       []domain.SnapshotID
	StaleIncompleteRuns      []domain.BackupRunID
	DestinationDivergence    []DestinationDifference
	Digest                   string
	StartedAt                time.Time
	CompletedAt              time.Time
}

type Scanner struct {
	Catalogue ports.Catalogue
	Store     ports.ObjectStore
	Clock     ports.Clock
}

func (s Scanner) Scan(ctx context.Context, repo domain.RepositoryID, mode Mode) (ScanResult, error) {
	start := s.Clock.Now()
	out := ScanResult{RepositoryID: repo, Mode: mode, StartedAt: start}
	if s.Store == nil {
		out.CompletedAt = s.Clock.Now()
		return out, nil
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		res, err := s.Store.List(ctx, ports.ListObjectsRequest{Prefix: "snapshots/", ContinuationToken: cursor, Limit: 1000})
		if err != nil {
			return out, err
		}
		for _, obj := range res.Objects {
			if strings.HasSuffix(obj.Key, "/complete.json") {
				out.ValidSnapshots++
				seen[obj.Key] = true
			}
		}
		if !res.HasMore {
			break
		}
		cursor = res.ContinuationToken
	}
	if s.Catalogue != nil {
		snaps, _ := s.Catalogue.ListSnapshots(ctx, "")
		for _, snap := range snaps {
			if snap.Status == domain.SnapshotCommitted && !seen[snap.CompletionObjectKey] {
				out.LocalOnlySnapshots = append(out.LocalOnlySnapshots, snap.ID)
			}
		}
	}
	out.CompletedAt = s.Clock.Now()
	return out, nil
}
