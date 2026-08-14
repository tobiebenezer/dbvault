package ports

import (
	"context"

	"github.com/dbvault/dbvault/internal/domain"
)

type AuthenticatedIdentity struct {
	UserID         domain.UserID
	OrganisationID domain.OrganisationID
	Groups         []string
	Claims         map[string]string
}

type IdentityProvider interface {
	Name() string
	Authenticate(context.Context, string) (AuthenticatedIdentity, error)
}

type SCIMProvisioner interface {
	CreateOrUpdateUser(context.Context, domain.OrganisationID, domain.UserID, map[string]string) error
	DeactivateUser(context.Context, domain.OrganisationID, domain.UserID) error
	SetGroups(context.Context, domain.OrganisationID, domain.UserID, []string) error
}
