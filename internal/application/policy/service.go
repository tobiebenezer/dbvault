package policy

import (
	"strings"
	"time"

	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
)

type Service struct{ Clock func() time.Time }

func (s Service) EvaluateRepository(scope domain.TenantScope, environmentClass string, repo config.RepositoryConfig) []domain.PolicyResult {
	now := time.Now().UTC()
	if s.Clock != nil {
		now = s.Clock()
	}
	result := []domain.PolicyResult{}
	add := func(id string, passed bool, message string) {
		result = append(result, domain.PolicyResult{RuleID: id, OrganisationID: scope.OrganisationID, ResourceType: "repository", ResourceID: repo.ID, Passed: passed, Message: message, EvaluatedAt: now})
	}
	if strings.EqualFold(environmentClass, "production") {
		destinations := 0
		if repo.Primary != nil {
			destinations++
		}
		destinations += len(repo.Mirrors) + len(repo.Replicas)
		add("production-two-destinations", destinations >= 2, "production repositories require at least two destinations")
		add("production-encryption", repo.Encryption.ActiveKey != "", "production repositories require an active encryption key")
		add("production-restore-retention", repo.Retention.MinimumVerifiedSnapshots >= 2, "production repositories require at least two verified snapshots")
	}
	return result
}
