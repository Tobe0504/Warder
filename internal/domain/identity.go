package domain

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// PrincipalType distinguishes the two kinds of authenticated subject the
// authorization engine understands. Everything else — whether a machine is a CI
// runner, an AI coding agent, or a production workload — is descriptive
// metadata carried in ActorType, not a separate authorization path.
type PrincipalType string

const (
	PrincipalUser    PrincipalType = "USER"
	PrincipalMachine PrincipalType = "MACHINE"
)

// ActorType records what kind of actor performed an action. It is used for
// audit and for presentation. It deliberately does not alter policy evaluation:
// an AI agent is constrained by its grants and token scope, not by a special
// case keyed on its type, so a new agent vendor never needs new policy code.
type ActorType string

const (
	ActorHuman    ActorType = "HUMAN"
	ActorService  ActorType = "SERVICE"
	ActorAIAgent  ActorType = "AI_AGENT"
	ActorCI       ActorType = "CI"
	ActorWorkload ActorType = "WORKLOAD"
)

// ValidActorType reports whether a is a recognized actor type.
func ValidActorType(a ActorType) bool {
	switch a {
	case ActorHuman, ActorService, ActorAIAgent, ActorCI, ActorWorkload:
		return true
	}
	return false
}

// MachineActorTypes are the actor types a machine identity may declare.
var MachineActorTypes = []ActorType{ActorService, ActorAIAgent, ActorCI, ActorWorkload}

// CredentialScope is the ceiling imposed by the specific credential used to
// authenticate, independent of what the underlying identity is allowed to do.
//
// Effective authority is always the intersection of the identity's grants and
// this scope. A token cannot widen an identity, only narrow it. That is what
// makes it safe to hand a long-lived identity a narrowly scoped token for one
// environment.
type CredentialScope struct {
	// ProjectID restricts the credential to a single project. Always set for
	// machine tokens; a token that names no project is rejected at mint time.
	ProjectID uuid.UUID

	// EnvironmentID restricts the credential to a single environment. A
	// development token is therefore structurally incapable of reaching
	// production, regardless of the identity's grants.
	EnvironmentID uuid.UUID

	// Capabilities is the capability ceiling of this credential.
	Capabilities []Capability

	// SecretKeys optionally narrows the credential to specific secret keys. An
	// empty slice means "no key-level restriction"; the environment-level
	// grants still apply.
	SecretKeys []string
}

// Allows reports whether the scope itself permits the capability.
func (s *CredentialScope) Allows(c Capability) bool {
	return slices.Contains(s.Capabilities, c)
}

// AllowsKey reports whether the scope permits this specific secret key.
func (s *CredentialScope) AllowsKey(key string) bool {
	if len(s.SecretKeys) == 0 {
		return true
	}
	return slices.Contains(s.SecretKeys, key)
}

// Principal is an authenticated subject together with the ceiling of the
// credential it presented. It is produced only by an IdentityProvider and is
// the sole input the rest of the system uses to answer "who is asking".
type Principal struct {
	Type           PrincipalType
	ID             uuid.UUID // user ID or machine identity ID
	OrganizationID uuid.UUID
	ActorType      ActorType

	// DisplayName is for audit and UI. It is untrusted, user-supplied text and
	// must never be interpolated into anything but escaped output.
	DisplayName string

	// Scope is non-nil when the credential is narrower than the identity, which
	// is the case for every machine token. A nil scope means the credential
	// carries the identity's full authority (an interactive browser session).
	Scope *CredentialScope

	// CredentialID identifies the token or session used, for audit and for
	// revocation traceability.
	CredentialID uuid.UUID

	// Role is the organization membership role, for user principals only.
	Role Role
}

// Organization is the top-level tenant boundary. No query in the system may
// cross it.
type Organization struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// User is a human account.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string // Argon2id PHC string; never leaves the store layer
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DisabledAt   *time.Time
}

// Membership binds a user to an organization with a role. ExpiresAt supports
// the contractor workflow: when it passes, the membership stops conferring
// anything, without any credential in the organization needing to be rotated.
type Membership struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Role           Role
	CreatedAt      time.Time
	CreatedBy      *uuid.UUID
	ExpiresAt      *time.Time
	RevokedAt      *time.Time
}

// Active reports whether the membership currently confers its role.
func (m *Membership) Active(now time.Time) bool {
	if m.RevokedAt != nil && !m.RevokedAt.After(now) {
		return false
	}
	if m.ExpiresAt != nil && !m.ExpiresAt.After(now) {
		return false
	}
	return true
}

// MachineIdentity is a non-human subject: an application, a CI pipeline, an AI
// coding agent session, or any other workload. It is a first-class identity
// with its own grants; it never inherits a developer's authority.
type MachineIdentity struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	ActorType      ActorType
	CreatedAt      time.Time
	CreatedBy      uuid.UUID
	DisabledAt     *time.Time

	// ExpiresAt bounds the lifetime of the identity itself, which is how an
	// agent session identity is made inherently temporary.
	ExpiresAt *time.Time
}

// Active reports whether the machine identity may still authenticate.
func (m *MachineIdentity) Active(now time.Time) bool {
	if m.DisabledAt != nil && !m.DisabledAt.After(now) {
		return false
	}
	if m.ExpiresAt != nil && !m.ExpiresAt.After(now) {
		return false
	}
	return true
}
