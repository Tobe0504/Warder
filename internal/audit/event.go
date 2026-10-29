// Package audit records security-relevant events.
//
// Two rules hold everywhere in this package. Events are append-only, enforced
// by the database. And no event may carry secret material: metadata passes
// through a scrubber on the way in, so an event recording that DATABASE_URL was
// used says exactly that and never what DATABASE_URL is.
package audit

import (
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

// EventType names a security-relevant action.
type EventType string

const (
	EventSecretCreated         EventType = "SECRET_CREATED"
	EventSecretRotated         EventType = "SECRET_ROTATED"
	EventSecretRolledBack      EventType = "SECRET_ROLLED_BACK"
	EventSecretRevoked         EventType = "SECRET_REVOKED"
	EventSecretExpiryChanged   EventType = "SECRET_EXPIRY_CHANGED"
	EventSecretDeleted         EventType = "SECRET_DELETED"
	EventSecretUsed            EventType = "SECRET_USED"
	EventSecretRevealRequested EventType = "SECRET_REVEAL_REQUESTED"
	EventSecretRevealed        EventType = "SECRET_REVEALED"

	EventTokenCreated EventType = "TOKEN_CREATED"
	EventTokenRevoked EventType = "TOKEN_REVOKED"
	EventTokenUsed    EventType = "TOKEN_USED"

	EventRuntimeAuthenticated EventType = "RUNTIME_AUTHENTICATED"

	EventAccessGranted EventType = "ACCESS_GRANTED"
	EventAccessRevoked EventType = "ACCESS_REVOKED"
	EventAccessDenied  EventType = "ACCESS_DENIED"

	EventIdentityCreated  EventType = "IDENTITY_CREATED"
	EventIdentityDisabled EventType = "IDENTITY_DISABLED"

	EventUserInvited        EventType = "USER_INVITED"
	EventUserRemoved        EventType = "USER_REMOVED"
	EventInvitationRevoked  EventType = "INVITATION_REVOKED"
	EventInvitationAccepted EventType = "INVITATION_ACCEPTED"
	EventInvitationRejected EventType = "INVITATION_REJECTED"

	EventProjectCreated     EventType = "PROJECT_CREATED"
	EventEnvironmentCreated EventType = "ENVIRONMENT_CREATED"

	EventLogin       EventType = "LOGIN"
	EventLoginFailed EventType = "LOGIN_FAILED"
	EventLogout      EventType = "LOGOUT"

	// EventOrganizationSwitched records a session being re-issued against a
	// different organization the same person belongs to. Recorded in both
	// organizations: the one being left has as much reason to know as the one
	// being entered.
	EventOrganizationSwitched EventType = "ORGANIZATION_SWITCHED"

	// EventOrganizationCreated records a person founding an organization and
	// becoming its first owner.
	EventOrganizationCreated EventType = "ORGANIZATION_CREATED"

	// EventPasswordChanged records a person replacing their own password while
	// signed in and able to prove the old one.
	EventPasswordChanged EventType = "PASSWORD_CHANGED"

	// EventPasswordReset records a password replaced by spending a recovery
	// code. Recorded on failure too: a run of denied resets against one
	// account is somebody working through guesses.
	EventPasswordReset EventType = "PASSWORD_RESET"

	// EventRecoveryCodesIssued records a new set being minted, which
	// invalidates every code from the set before it.
	EventRecoveryCodesIssued EventType = "RECOVERY_CODES_ISSUED"

	// EventPasskeyRegistered records a new second factor being enrolled.
	EventPasskeyRegistered EventType = "PASSKEY_REGISTERED"

	// EventPasskeyRemoved records one being removed, which may be the last and
	// may therefore drop an account back to a password alone.
	EventPasskeyRemoved EventType = "PASSKEY_REMOVED"

	// EventAccountRecoveryIssued records an owner minting a link that lets
	// somebody else set a password. The loudest event in the account
	// lifecycle, because whoever redeems it can then sign in as that person.
	EventAccountRecoveryIssued EventType = "ACCOUNT_RECOVERY_ISSUED"

	// EventPasskeyRejected records a refused assertion. A cloned authenticator
	// shows up here and nowhere else.
	EventPasskeyRejected EventType = "PASSKEY_REJECTED"
	EventRateLimited     EventType = "RATE_LIMITED"
	EventDecryptError    EventType = "DECRYPTION_FAILED"
)

// Outcome is the result of the audited action.
type Outcome string

const (
	OutcomeSuccess Outcome = "SUCCESS"
	OutcomeDenied  Outcome = "DENIED"
	OutcomeFailure Outcome = "FAILURE"
)

// Event is one audit record.
//
// There is no field here capable of holding a secret value, and Metadata is
// scrubbed before it is written. Recording that a credential was used is the
// entire point; recording the credential would defeat it.
type Event struct {
	OrganizationID uuid.UUID
	OccurredAt     time.Time

	Type    EventType
	Outcome Outcome

	ActorType domain.ActorType
	ActorID   *uuid.UUID
	// ActorLabel is denormalized so the trail stays readable after the actor is
	// deleted. A log of unresolvable identifiers is not an audit trail.
	ActorLabel   string
	CredentialID *uuid.UUID

	ProjectID     *uuid.UUID
	EnvironmentID *uuid.UUID
	SecretID      *uuid.UUID
	// SecretKey is the name, never the value.
	SecretKey string
	TokenID   *uuid.UUID

	IPAddress string
	UserAgent string

	// Reason explains a denial in terms safe to show an administrator.
	Reason string

	Metadata map[string]any
}
