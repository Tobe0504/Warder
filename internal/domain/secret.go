package domain

import (
	"time"

	"github.com/google/uuid"
)

// Project groups environments within an organization.
type Project struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Slug           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Environment is the unit at which access is normally granted. Environments are
// not ranked by the policy engine — "production" holds no special meaning in
// code. Isolation comes from grants and token scopes naming a specific
// environment ID, which is what makes custom environments (preview, qa,
// sandbox) exactly as safe as the built-in ones.
type Environment struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Secret is metadata only. It deliberately has no value field: plaintext, and
// even ciphertext, live in SecretVersion. A dump of this table reveals which
// credentials exist, never what they are.
type Secret struct {
	ID            uuid.UUID
	EnvironmentID uuid.UUID
	Key           string
	Description   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CreatedBy     uuid.UUID
	DeletedAt     *time.Time

	// LastUsedAt answers the administrator's question "is anything still using
	// this?" before they revoke it. It is updated on successful use.
	LastUsedAt *time.Time
}

// VersionStatus is the lifecycle state of a single secret version.
type VersionStatus string

const (
	// VersionActive is the version delivered to authorized runtimes. At most
	// one version per secret is active at a time.
	VersionActive VersionStatus = "ACTIVE"

	// VersionSuperseded is a previous version retained for rollback.
	VersionSuperseded VersionStatus = "SUPERSEDED"

	// VersionRevoked is a version that must never be delivered again.
	VersionRevoked VersionStatus = "REVOKED"
)

// SecretVersion carries the encrypted material for one revision of a secret.
//
// The ciphertext fields are never populated by list queries; they are loaded
// only by the one repository method used after an authorization decision has
// already been made. See store.SecretRepo.
type SecretVersion struct {
	ID        uuid.UUID
	SecretID  uuid.UUID
	Version   int
	Status    VersionStatus
	CreatedAt time.Time
	CreatedBy uuid.UUID
	ExpiresAt *time.Time
	RevokedAt *time.Time

	// EncryptionKeyID records which key encryption key version wrapped this
	// version's data key, so that keys can be rotated without re-encrypting
	// everything at once.
	EncryptionKeyID string
}

// Deliverable reports whether this version may be released to a runtime.
// Expiry and revocation are enforced here, in the domain, so that every caller
// gets the same answer rather than each re-deriving it.
func (v *SecretVersion) Deliverable(now time.Time) bool {
	if v.Status != VersionActive {
		return false
	}
	if v.RevokedAt != nil && !v.RevokedAt.After(now) {
		return false
	}
	if v.Expired(now) {
		return false
	}
	return true
}

// Expired reports whether the version has passed its expiry time.
func (v *SecretVersion) Expired(now time.Time) bool {
	return v.ExpiresAt != nil && !v.ExpiresAt.After(now)
}

// SubjectType identifies what kind of identity an access grant is attached to.
type SubjectType string

const (
	SubjectUser    SubjectType = "USER"
	SubjectMachine SubjectType = "MACHINE"
)

// ValidSubjectType reports whether s is recognized.
func ValidSubjectType(s SubjectType) bool {
	return s == SubjectUser || s == SubjectMachine
}

// AccessGrant is the explicit, auditable statement that a specific identity may
// exercise specific capabilities over a specific slice of the secret tree.
//
// Scope narrows left to right: an organization-wide grant leaves ProjectID nil,
// a project-wide grant leaves EnvironmentID nil, and so on. The API layer
// requires callers to opt into wildcards explicitly rather than producing them
// by omitting a field, so a missing form value can never widen a grant.
type AccessGrant struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID

	SubjectType SubjectType
	SubjectID   uuid.UUID

	ProjectID     *uuid.UUID // nil = every project in the organization
	EnvironmentID *uuid.UUID // nil = every environment in the project
	SecretID      *uuid.UUID // nil = every secret in the environment

	Capabilities []Capability

	CreatedAt time.Time
	CreatedBy uuid.UUID
	ExpiresAt *time.Time
	RevokedAt *time.Time

	// Reason records why the grant exists. It matters most for READ_SECRET,
	// where an auditor's question is not "who can see this" but "why".
	Reason string
}

// Active reports whether the grant currently confers anything.
func (g *AccessGrant) Active(now time.Time) bool {
	if g.RevokedAt != nil && !g.RevokedAt.After(now) {
		return false
	}
	if g.ExpiresAt != nil && !g.ExpiresAt.After(now) {
		return false
	}
	return true
}
