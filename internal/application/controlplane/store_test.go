package controlplane

import (
	"errors"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestTenantIsolation(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()
	orgA, orgB := domain.OrganisationID("org-a"), domain.OrganisationID("org-b")
	_ = store.CreateOrganisation(domain.Organisation{ID: orgA, Name: "A", Slug: "a", Status: domain.OrganisationActive, CreatedAt: now})
	_ = store.CreateOrganisation(domain.Organisation{ID: orgB, Name: "B", Slug: "b", Status: domain.OrganisationActive, CreatedAt: now})
	scopeA := domain.TenantScope{OrganisationID: orgA, ProjectID: "project-a"}
	if err := store.CreateProject(scopeA, domain.Project{ID: "project-a", OrganisationID: orgA, Name: "A", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterAgent(scopeA, domain.Agent{ID: "agent-a", OrganisationID: orgA, ProjectID: "project-a", Status: domain.AgentOnline}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSource(scopeA, ManagedSource{ID: "source-a", OrganisationID: orgA, ProjectID: "project-a", AgentID: "agent-a"}); err != nil {
		t.Fatal(err)
	}
	_, err := store.GetSource(domain.TenantScope{OrganisationID: orgB, ProjectID: "project-b"}, "source-a")
	if !errors.Is(err, ErrTenantIsolation) {
		t.Fatalf("expected tenant isolation, got %v", err)
	}
}
