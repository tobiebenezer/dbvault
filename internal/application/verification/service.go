package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Service struct {
	Catalogue ports.Catalogue
	Store     ports.ObjectStore
	Signer    ports.ManifestSigner
}
type Result struct {
	SnapshotID domain.SnapshotID
	Levels     []string
	ChunkCount int
}

func (s Service) VerifyRemote(ctx context.Context, id domain.SnapshotID) (Result, error) {
	snap, _, err := s.Catalogue.GetSnapshot(ctx, id)
	if err != nil {
		return Result{}, err
	}
	r, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: snap.ManifestObjectKey})
	if err != nil {
		return Result{}, err
	}
	payload, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		return Result{}, err
	}
	sigR, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: fmt.Sprintf("snapshots/%s/%s/manifest.sig", snap.SourceID, snap.ID)})
	if err != nil {
		return Result{}, err
	}
	sig, err := io.ReadAll(sigR)
	_ = sigR.Close()
	if err != nil {
		return Result{}, err
	}
	if err := s.Signer.Verify(payload, sig); err != nil {
		return Result{}, err
	}
	var man mf.SnapshotManifest
	if err := json.Unmarshal(payload, &man); err != nil {
		return Result{}, err
	}
	for _, ch := range man.Chunks {
		if _, err := s.Store.Head(ctx, ch.ObjectKey); err != nil {
			return Result{}, err
		}
	}
	return Result{SnapshotID: id, Levels: []string{"publication", "signature", "chunk-existence"}, ChunkCount: len(man.Chunks)}, nil
}
