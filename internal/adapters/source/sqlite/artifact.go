package sqlite

import (
	"github.com/dbvault/dbvault/internal/ports"
	"os"
)

type Artifact struct {
	path    string
	size    int64
	meta    ports.SnapshotMetadata
	cleanup bool
}

func (a *Artifact) Path() string                     { return a.path }
func (a *Artifact) Size() int64                      { return a.size }
func (a *Artifact) Metadata() ports.SnapshotMetadata { return a.meta }
func (a *Artifact) Close() error {
	if a.cleanup {
		return os.Remove(a.path)
	}
	return nil
}
