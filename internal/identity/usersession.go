package identity

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

// UserSessionProvider authenticates a human, from either a browser session
// cookie relayed by the BFF or a CLI login.
//
// A user session carries no credential scope: it represents the person's full
// authority, which their grants then bound. That is the difference between a
// human session and a machine token, and it is why `ward run` exchanges a user
// session for a scoped runtime session naming one project and environment
// rather than fetching secrets with the session directly.
type UserSessionProvider struct {
	accounts *store.AccountRepo
	now      func() time.Time

	// allowedKinds restricts which session kinds this provider will accept.
	//
	// Browser sessions and CLI logins are both user sessions, but they are
	// issued to different places and reach the system through different
	// surfaces. Keeping them apart means a browser session token — the one that
	// sits in a cookie and is exposed to any cross-site scripting flaw in the
	// dashboard — cannot be replayed against the secret delivery API, and a CLI
	// login sitting in a file on a laptop cannot be used to administer the
	// organization.
	allowedKinds []string
}

// NewUserSessionProvider constructs the provider. Passing no kinds accepts any.
func NewUserSessionProvider(accounts *store.AccountRepo, now func() time.Time, allowedKinds ...string) *UserSessionProvider {
	if now == nil {
		now = time.Now
	}
	return &UserSessionProvider{accounts: accounts, now: now, allowedKinds: allowedKinds}
}

func (p *UserSessionProvider) kindAllowed(kind string) bool {
	if len(p.allowedKinds) == 0 {
		return true
	}
	return slices.Contains(p.allowedKinds, kind)
}

// Name implements Provider.
func (p *UserSessionProvider) Name() string { return "user-session" }

// Handles implements Provider.
func (p *UserSessionProvider) Handles(req Request) bool {
	return strings.HasPrefix(req.Credential, string(credential.KindSession)+"_")
}

// Authenticate implements Provider.
func (p *UserSessionProvider) Authenticate(ctx context.Context, req Request) (*domain.Principal, error) {
	kind, publicID, err := credential.Parse(req.Credential)
	if err != nil || kind != credential.KindSession {
		return nil, ErrUnauthenticated
	}

	candidate, err := p.accounts.FindSessionByPublicID(ctx, publicID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}

	if !credential.Verify(req.Credential, candidate.TokenHash) {
		return nil, ErrUnauthenticated
	}

	if !p.kindAllowed(candidate.Kind) {
		return nil, ErrUnauthenticated
	}

	now := p.now()
	if candidate.RevokedAt != nil && !candidate.RevokedAt.After(now) {
		return nil, ErrUnauthenticated
	}
	if !candidate.ExpiresAt.After(now) {
		return nil, ErrUnauthenticated
	}

	// The membership is re-read on every request rather than baked into the
	// session at login. This is what makes the contractor workflow work: when a
	// membership expires or is revoked, sessions already in flight stop
	// conferring anything on their very next request, with no credential in the
	// organization needing to change.
	membership, err := p.accounts.GetMembership(ctx, candidate.OrganizationID, candidate.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}
	if !membership.Active(now) {
		return nil, ErrUnauthenticated
	}

	user, err := p.accounts.GetUser(ctx, candidate.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}
	if user.DisabledAt != nil && !user.DisabledAt.After(now) {
		return nil, ErrUnauthenticated
	}

	return &domain.Principal{
		Type:           domain.PrincipalUser,
		ID:             candidate.UserID,
		OrganizationID: candidate.OrganizationID,
		ActorType:      domain.ActorHuman,
		DisplayName:    user.Name,
		CredentialID:   candidate.ID,
		Role:           membership.Role,
	}, nil
}
