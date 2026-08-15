package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

// MachineTokenProvider authenticates long-lived runtime tokens.
//
// This is the MVP mechanism, and it is the weakest link in the runtime story: a
// long-lived bearer token is only as safe as the place it is stored. The
// architecture treats it as one implementation among several rather than as the
// model, which is why POST /runtime/auth immediately exchanges it for a
// short-lived session and why this type implements a general interface.
type MachineTokenProvider struct {
	machines *store.MachineRepo
	now      func() time.Time
}

// NewMachineTokenProvider constructs the provider.
func NewMachineTokenProvider(machines *store.MachineRepo, now func() time.Time) *MachineTokenProvider {
	if now == nil {
		now = time.Now
	}
	return &MachineTokenProvider{machines: machines, now: now}
}

// Name implements Provider.
func (p *MachineTokenProvider) Name() string { return "machine-token" }

// Handles implements Provider.
func (p *MachineTokenProvider) Handles(req Request) bool {
	return strings.HasPrefix(req.Credential, string(credential.KindMachine)+"_")
}

// Authenticate implements Provider.
//
// The order of checks is: parse, find the candidate row by its public handle,
// verify the secret half in constant time, then evaluate the lifecycle of both
// the token and the identity behind it. Verification happens before the
// lifecycle checks so that timing does not distinguish "revoked token" from
// "token that never existed".
func (p *MachineTokenProvider) Authenticate(ctx context.Context, req Request) (*domain.Principal, error) {
	kind, publicID, err := credential.Parse(req.Credential)
	if err != nil || kind != credential.KindMachine {
		return nil, ErrUnauthenticated
	}

	candidate, err := p.machines.FindTokenByPublicID(ctx, publicID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}

	if !credential.Verify(req.Credential, candidate.TokenHash) {
		return nil, ErrUnauthenticated
	}

	now := p.now()
	if !candidate.Active(now) {
		return nil, ErrUnauthenticated
	}

	// The identity behind the token must also still be active. Disabling an
	// identity has to stop every token it issued, without an administrator
	// having to find and revoke each one; that is what makes offboarding an
	// agent or a contractor a single action.
	if candidate.IdentityDisabledAt != nil && !candidate.IdentityDisabledAt.After(now) {
		return nil, ErrUnauthenticated
	}
	if candidate.IdentityExpiresAt != nil && !candidate.IdentityExpiresAt.After(now) {
		return nil, ErrUnauthenticated
	}

	return &domain.Principal{
		Type:           domain.PrincipalMachine,
		ID:             candidate.MachineIdentityID,
		OrganizationID: candidate.OrganizationID,
		ActorType:      candidate.IdentityActorType,
		DisplayName:    candidate.IdentityName,
		CredentialID:   candidate.ID,
		Scope: &domain.CredentialScope{
			ProjectID:     candidate.ProjectID,
			EnvironmentID: candidate.EnvironmentID,
			Capabilities:  candidate.Capabilities,
			SecretKeys:    candidate.SecretKeys,
		},
	}, nil
}

// RuntimeSessionProvider authenticates the short-lived credential minted by
// POST /runtime/auth and presented to POST /runtime/secrets.
//
// Splitting the two means the long-lived token is transmitted once per process
// start rather than on every secret retrieval, and that the credential which
// actually accompanies secret delivery expires in minutes.
type RuntimeSessionProvider struct {
	machines *store.MachineRepo
	now      func() time.Time
}

// NewRuntimeSessionProvider constructs the provider.
func NewRuntimeSessionProvider(machines *store.MachineRepo, now func() time.Time) *RuntimeSessionProvider {
	if now == nil {
		now = time.Now
	}
	return &RuntimeSessionProvider{machines: machines, now: now}
}

// Name implements Provider.
func (p *RuntimeSessionProvider) Name() string { return "runtime-session" }

// Handles implements Provider.
func (p *RuntimeSessionProvider) Handles(req Request) bool {
	return strings.HasPrefix(req.Credential, string(credential.KindRuntime)+"_")
}

// Authenticate implements Provider.
func (p *RuntimeSessionProvider) Authenticate(ctx context.Context, req Request) (*domain.Principal, error) {
	kind, publicID, err := credential.Parse(req.Credential)
	if err != nil || kind != credential.KindRuntime {
		return nil, ErrUnauthenticated
	}

	candidate, err := p.machines.FindRuntimeSessionByPublicID(ctx, publicID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}

	if !credential.Verify(req.Credential, candidate.TokenHash) {
		return nil, ErrUnauthenticated
	}

	now := p.now()
	if candidate.RevokedAt != nil && !candidate.RevokedAt.After(now) {
		return nil, ErrUnauthenticated
	}
	if !candidate.ExpiresAt.After(now) {
		return nil, ErrUnauthenticated
	}

	principalType := domain.PrincipalMachine
	if candidate.SubjectType == domain.SubjectUser {
		principalType = domain.PrincipalUser
	}

	return &domain.Principal{
		Type:           principalType,
		ID:             candidate.SubjectID,
		OrganizationID: candidate.OrganizationID,
		ActorType:      candidate.ActorType,
		CredentialID:   candidate.ID,
		Scope: &domain.CredentialScope{
			ProjectID:     candidate.ProjectID,
			EnvironmentID: candidate.EnvironmentID,
			Capabilities:  candidate.Capabilities,
			SecretKeys:    candidate.SecretKeys,
		},
	}, nil
}
