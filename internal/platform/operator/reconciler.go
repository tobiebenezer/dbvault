package operator

import (
	"fmt"
	"sync"

	platformk8s "github.com/dbvault/dbvault/internal/platform/kubernetes"
)

type Status struct {
	Ready      bool   `json:"ready"`
	Message    string `json:"message"`
	Generation int64  `json:"generation"`
}

type Reconciler struct {
	mu          sync.Mutex
	generations map[string]int64
}

func New() *Reconciler { return &Reconciler{generations: map[string]int64{}} }
func (r *Reconciler) ReconcileSource(source platformk8s.DBVaultSource) (Status, error) {
	if err := platformk8s.ValidateSource(source); err != nil {
		return Status{Message: err.Error()}, err
	}
	key := source.Metadata.Namespace + "/" + source.Metadata.Name
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generations[key]++
	return Status{Ready: true, Message: "source accepted for agent reconciliation", Generation: r.generations[key]}, nil
}
func (r *Reconciler) ReconcileRepository(repository platformk8s.DBVaultRepository) (Status, error) {
	if err := platformk8s.ValidateRepository(repository); err != nil {
		return Status{Message: err.Error()}, err
	}
	key := repository.Metadata.Namespace + "/" + repository.Metadata.Name
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generations[key]++
	return Status{Ready: true, Message: fmt.Sprintf("repository %s accepted", repository.Metadata.Name), Generation: r.generations[key]}, nil
}
