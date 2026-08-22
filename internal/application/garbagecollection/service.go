package garbagecollection

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Plan struct {
	ID               domain.GCPlanID
	RepositoryID     domain.RepositoryID
	RepositoryDigest string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	ReclaimableBytes int64
	DeleteKeys       []string
	Blocked          []string
	ScannedObjects   int
	ReferencedChunks int
}

type Service struct {
	Catalogue ports.Catalogue
	Store     ports.ObjectStore
	Clock     ports.Clock
}

// Plan performs a full orphan sweep:
//  1. List all chunk objects in the store (prefix "chunks/")
//  2. List all committed snapshot manifests (prefix "snapshots/")
//  3. Any chunk not referenced by any manifest → candidate for deletion
func (s Service) Plan(ctx context.Context, repo domain.RepositoryID) (Plan, error) {
	now := s.Clock.Now()
	plan := Plan{
		ID:           domain.GCPlanID("gc_" + now.Format("20060102150405")),
		RepositoryID: repo,
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Hour),
	}

	if s.Store == nil {
		return plan, nil
	}

	// Step 1: collect all chunk object keys in the store
	chunkKeys := make(map[string]int64) // key → size
	token := ""
	for {
		res, err := s.Store.List(ctx, ports.ListObjectsRequest{Prefix: "chunks/", ContinuationToken: token, Limit: 1000})
		if err != nil {
			return plan, err
		}
		for _, obj := range res.Objects {
			chunkKeys[obj.Key] = obj.Size
			plan.ScannedObjects++
		}
		if !res.HasMore {
			break
		}
		token = res.ContinuationToken
	}

	// Step 2: collect all referenced chunk keys from every snapshot manifest
	referenced := make(map[string]struct{})
	manifestToken := ""
	for {
		res, err := s.Store.List(ctx, ports.ListObjectsRequest{Prefix: "snapshots/", ContinuationToken: manifestToken, Limit: 1000})
		if err != nil {
			return plan, err
		}
		for _, obj := range res.Objects {
			if !strings.HasSuffix(obj.Key, "/manifest.json") {
				continue
			}
			r, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: obj.Key})
			if err != nil {
				plan.Blocked = append(plan.Blocked, obj.Key)
				continue
			}
			payload, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				plan.Blocked = append(plan.Blocked, obj.Key)
				continue
			}
			var man mf.SnapshotManifest
			if err := json.Unmarshal(payload, &man); err != nil {
				plan.Blocked = append(plan.Blocked, obj.Key)
				continue
			}
			for _, ch := range man.Chunks {
				referenced[ch.ObjectKey] = struct{}{}
				plan.ReferencedChunks++
			}
		}
		if !res.HasMore {
			break
		}
		manifestToken = res.ContinuationToken
	}

	// Step 3: any chunk key not in referenced set is an orphan
	for key, size := range chunkKeys {
		if _, ok := referenced[key]; !ok {
			plan.DeleteKeys = append(plan.DeleteKeys, key)
			plan.ReclaimableBytes += size
		}
	}

	return plan, nil
}

func (s Service) Run(ctx context.Context, plan Plan) error {
	if !plan.ExpiresAt.IsZero() && s.Clock.Now().After(plan.ExpiresAt) {
		return domain.NewError(domain.ErrGCPlanStale, "garbage-collection plan expired", nil)
	}
	for _, k := range plan.DeleteKeys {
		if err := s.Store.Delete(ctx, k); err != nil {
			return err
		}
	}
	return nil
}
