package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Service struct {
	Catalogue ports.Catalogue
	Primary   ports.ObjectStore
	Replicas  map[domain.DestinationID]ports.ObjectStore
	Signer    ports.ManifestSigner
}

type Result struct {
	SnapshotID    domain.SnapshotID
	DestinationID domain.DestinationID
	CopiedObjects int
	ReusedObjects int
}

func (s *Service) Replicate(ctx context.Context, snapshotID domain.SnapshotID, destinationID domain.DestinationID) (Result, error) {
	if s.Primary == nil {
		return Result{}, domain.NewError(domain.ErrConfigurationInvalid, "primary store missing", nil)
	}
	target := s.Replicas[destinationID]
	if target == nil {
		return Result{}, domain.NewError(domain.ErrConfigurationInvalid, "replica store missing", nil)
	}
	snap, _, err := s.Catalogue.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return Result{}, err
	}
	payload, err := readObject(ctx, s.Primary, snap.ManifestObjectKey)
	if err != nil {
		return Result{}, err
	}
	var man mf.SnapshotManifest
	if err := json.Unmarshal(payload, &man); err != nil {
		return Result{}, err
	}
	res := Result{SnapshotID: snapshotID, DestinationID: destinationID}
	for _, ch := range man.Chunks {
		if _, err := target.Head(ctx, ch.ObjectKey); err == nil {
			res.ReusedObjects++
			continue
		}
		if err := copyObject(ctx, s.Primary, target, ch.ObjectKey); err != nil {
			return res, err
		}
		res.CopiedObjects++
	}
	sigKey := strings.TrimSuffix(snap.ManifestObjectKey, ".json") + ".sig"
	if sigKey == "" || sigKey == ".sig" {
		sigKey = fmt.Sprintf("%s/snapshots/%s/manifest.sig", snap.SourceID, snap.ID)
	}
	for _, key := range []string{snap.ManifestObjectKey, sigKey, snap.CompletionObjectKey} {
		if err := copyObject(ctx, s.Primary, target, key); err != nil {
			return res, err
		}
	}
	return res, nil
}

func copyObject(ctx context.Context, src, dst ports.ObjectStore, key string) error {
	r, obj, err := src.Get(ctx, ports.GetObjectRequest{Key: key})
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = dst.Put(ctx, ports.PutObjectRequest{Key: key, Body: r, Size: obj.Size, ContentType: "application/octet-stream", Metadata: obj.Metadata, IfNotExists: true})
	return err
}
func readObject(ctx context.Context, store ports.ObjectStore, key string) ([]byte, error) {
	r, _, err := store.Get(ctx, ports.GetObjectRequest{Key: key})
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
