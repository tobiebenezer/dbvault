package controlplane

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

var ErrTenantIsolation = errors.New("resource is outside the caller tenant scope")

type ManagedSource struct {
	ID             string                `json:"id"`
	OrganisationID domain.OrganisationID `json:"organisation_id"`
	ProjectID      domain.ProjectID      `json:"project_id"`
	EnvironmentID  domain.EnvironmentID  `json:"environment_id"`
	Engine         string                `json:"engine"`
	RepositoryID   string                `json:"repository_id"`
	AgentID        domain.AgentID        `json:"agent_id"`
	CreatedAt      time.Time             `json:"created_at"`
}

type Store struct {
	mu            sync.RWMutex
	organisations map[domain.OrganisationID]domain.Organisation
	projects      map[domain.ProjectID]domain.Project
	environments  map[domain.EnvironmentID]domain.Environment
	agents        map[domain.AgentID]domain.Agent
	sources       map[string]ManagedSource
}

func NewStore() *Store {
	return &Store{organisations: map[domain.OrganisationID]domain.Organisation{}, projects: map[domain.ProjectID]domain.Project{}, environments: map[domain.EnvironmentID]domain.Environment{}, agents: map[domain.AgentID]domain.Agent{}, sources: map[string]ManagedSource{}}
}

func (s *Store) CreateOrganisation(org domain.Organisation) error {
	if org.ID == "" || org.Name == "" {
		return errors.New("organisation id and name are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.organisations[org.ID]; exists {
		return fmt.Errorf("organisation %s already exists", org.ID)
	}
	s.organisations[org.ID] = org
	return nil
}

func (s *Store) CreateProject(scope domain.TenantScope, project domain.Project) error {
	if !scope.Valid() || project.OrganisationID != scope.OrganisationID {
		return ErrTenantIsolation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organisations[scope.OrganisationID]; !ok {
		return errors.New("organisation not found")
	}
	if _, exists := s.projects[project.ID]; exists {
		return fmt.Errorf("project %s already exists", project.ID)
	}
	s.projects[project.ID] = project
	return nil
}

func (s *Store) CreateEnvironment(scope domain.TenantScope, environment domain.Environment) error {
	if environment.OrganisationID != scope.OrganisationID || environment.ProjectID != scope.ProjectID {
		return ErrTenantIsolation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	project, ok := s.projects[environment.ProjectID]
	if !ok || project.OrganisationID != scope.OrganisationID {
		return ErrTenantIsolation
	}
	s.environments[environment.ID] = environment
	return nil
}

func (s *Store) RegisterAgent(scope domain.TenantScope, agent domain.Agent) error {
	if agent.OrganisationID != scope.OrganisationID || (scope.ProjectID != "" && agent.ProjectID != scope.ProjectID) {
		return ErrTenantIsolation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agent.ID] = agent
	return nil
}

func (s *Store) PutSource(scope domain.TenantScope, source ManagedSource) error {
	if source.OrganisationID != scope.OrganisationID || source.ProjectID != scope.ProjectID || (scope.EnvironmentID != "" && source.EnvironmentID != scope.EnvironmentID) {
		return ErrTenantIsolation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if agent, ok := s.agents[source.AgentID]; !ok || agent.OrganisationID != scope.OrganisationID {
		return ErrTenantIsolation
	}
	s.sources[source.ID] = source
	return nil
}

func (s *Store) GetSource(scope domain.TenantScope, id string) (ManagedSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.sources[id]
	if !ok {
		return ManagedSource{}, errors.New("source not found")
	}
	if source.OrganisationID != scope.OrganisationID || (scope.ProjectID != "" && source.ProjectID != scope.ProjectID) {
		return ManagedSource{}, ErrTenantIsolation
	}
	return source, nil
}

func (s *Store) ListSources(scope domain.TenantScope) []ManagedSource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []ManagedSource{}
	for _, source := range s.sources {
		if source.OrganisationID == scope.OrganisationID && (scope.ProjectID == "" || source.ProjectID == scope.ProjectID) {
			out = append(out, source)
		}
	}
	return out
}

func (s *Store) Heartbeat(scope domain.TenantScope, agentID domain.AgentID, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	agent, ok := s.agents[agentID]
	if !ok || agent.OrganisationID != scope.OrganisationID {
		return ErrTenantIsolation
	}
	agent.Status = domain.AgentOnline
	agent.LastSeenAt = &now
	agent.UpdatedAt = now
	s.agents[agentID] = agent
	return nil
}
