// Package secrets is the domain service that mediates every path plaintext can
// travel. Creation, rotation, runtime delivery, and human reveal all go through
// here, so the ordering of authorize, decrypt, and audit is written once.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/authz"
	"github.com/Tobe0504/Warder/internal/crypto"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/secretvalue"
	"github.com/Tobe0504/Warder/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MaxSecretBytes bounds a stored value. It is generous enough for certificates
// and private keys and small enough that a single request cannot be used to
// push arbitrary bulk data into the encrypted store.
const MaxSecretBytes = 512 * 1024

var (
	// ErrNotAuthorized is returned when policy refuses an operation.
	ErrNotAuthorized = errors.New("not authorized")

	// ErrSecretUnavailable is returned when a secret exists but has no
	// deliverable version: expired, revoked, or never given a value.
	ErrSecretUnavailable = errors.New("secret is not available")

	// ErrNotFound is returned when a secret does not exist.
	ErrNotFound = errors.New("secret not found")

	// ErrValueTooLarge is returned for oversized input.
	ErrValueTooLarge = errors.New("secret value is too large")
)

// Service coordinates authorization, encryption, storage, and audit.
type Service struct {
	db      *store.DB
	secrets *store.SecretRepo
	crypto  crypto.SecretEncryptionService
	policy  *authz.Engine
	audit   audit.Recorder
	logger  *slog.Logger
	now     func() time.Time
}

// Config supplies the service's dependencies.
type Config struct {
	DB      *store.DB
	Secrets *store.SecretRepo
	Crypto  crypto.SecretEncryptionService
	Policy  *authz.Engine
	Audit   audit.Recorder
	Logger  *slog.Logger
	Now     func() time.Time
}

// NewService constructs the service.
func NewService(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		db:      cfg.DB,
		secrets: cfg.Secrets,
		crypto:  cfg.Crypto,
		policy:  cfg.Policy,
		audit:   cfg.Audit,
		logger:  cfg.Logger,
		now:     now,
	}
}

// RequestContext carries the caller and the transport facts recorded in audit.
type RequestContext struct {
	Principal domain.Principal
	ClientIP  string
	UserAgent string
}

// ---------------------------------------------------------------------------
// Runtime delivery
// ---------------------------------------------------------------------------

// DeliverRequest asks for the values a runtime needs.
type DeliverRequest struct {
	ProjectID     uuid.UUID
	EnvironmentID uuid.UUID

	// Keys narrows the request to the secrets the runtime actually needs. An
	// empty slice means every secret in the environment the caller is
	// authorized for, which is the fallback for a runtime that did not narrow;
	// naming keys is what keeps the blast radius of one compromised process
	// down to what that process legitimately uses.
	Keys []string
}

// Delivery is the result of a runtime request.
type Delivery struct {
	// Values holds the plaintext, wrapped so it cannot be logged or serialized
	// by accident. The caller must call Expose to render it.
	Values map[string]secretvalue.Value

	// Denied lists requested keys the caller may not use. It carries no
	// information about whether the key exists, only that the caller cannot
	// have it.
	Denied []string

	// Unavailable lists requested keys the caller is authorized for but which
	// have no deliverable version, expired or revoked. Distinguishing this
	// from Denied is safe because the caller has already proven authorization
	// over the key, and it turns a mystifying outage into a legible one.
	Unavailable []string
}

// Deliver authorizes, decrypts, and returns secret values for a runtime.
//
// The sequence is the security-critical part of this system:
//
//  1. resolve the requested keys to metadata only, reading no ciphertext;
//  2. ask the policy engine about each key individually;
//  3. check that the version is actually deliverable, not expired or revoked;
//  4. load and decrypt only the material that survived all three;
//  5. audit each key, whether it was delivered or denied.
//
// Nothing is decrypted before step 4. A caller who is authorized for one key in
// an environment causes exactly one decryption, not one per secret present.
func (s *Service) Deliver(ctx context.Context, rc RequestContext, req DeliverRequest) (*Delivery, error) {
	// Step 1: metadata only. This query cannot return ciphertext.
	candidates, err := s.secrets.ResolveForDelivery(ctx, req.EnvironmentID, req.Keys)
	if err != nil {
		return nil, fmt.Errorf("secrets: resolving requested keys: %w", err)
	}

	found := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		found[c.Key] = true
	}

	result := &Delivery{Values: make(map[string]secretvalue.Value)}
	now := s.now()
	var used []uuid.UUID

	for i := range candidates {
		c := &candidates[i]

		// Step 2: authorize this specific key.
		decision, err := s.policy.Authorize(ctx, authz.Request{
			Principal:      rc.Principal,
			Capability:     domain.CapUseSecret,
			OrganizationID: rc.Principal.OrganizationID,
			ProjectID:      req.ProjectID,
			EnvironmentID:  req.EnvironmentID,
			SecretID:       c.ID,
			SecretKey:      c.Key,
		})
		if err != nil {
			return nil, fmt.Errorf("secrets: evaluating policy: %w", err)
		}
		if !decision.Allowed {
			result.Denied = append(result.Denied, c.Key)
			s.recordDenied(ctx, rc, c, req, decision)
			continue
		}

		// Step 3: is this version releasable at all?
		if !c.Deliverable(now) {
			result.Unavailable = append(result.Unavailable, c.Key)
			s.audit.Record(ctx, s.event(rc, audit.EventSecretUsed, audit.OutcomeFailure, c, req,
				unavailableReason(c, now), nil))
			continue
		}

		// Step 4: only now is any ciphertext read or opened.
		value, err := s.decrypt(ctx, rc.Principal.OrganizationID, req.ProjectID, req.EnvironmentID, c)
		if err != nil {
			// The cause is logged for operators and never returned to the
			// caller: a decryption failure that describes itself is an oracle.
			s.logger.Error("secret material could not be decrypted",
				"secret_key", c.Key, "secret_id", c.ID.String(),
				"environment_id", req.EnvironmentID.String(), "error", err)
			s.audit.Record(ctx, s.event(rc, audit.EventDecryptError, audit.OutcomeFailure, c, req,
				"secret material could not be decrypted", nil))
			result.Unavailable = append(result.Unavailable, c.Key)
			continue
		}

		result.Values[c.Key] = value
		used = append(used, c.ID)

		// Step 5: record the use. The key is recorded, the value never is.
		s.audit.Record(ctx, s.event(rc, audit.EventSecretUsed, audit.OutcomeSuccess, c, req, "", map[string]any{
			"version": derefInt(c.Version),
		}))
	}

	// A requested key that does not exist is reported as denied rather than as
	// missing. Otherwise the response distinguishes "no such secret" from "not
	// yours", and an unauthorized caller could enumerate an environment's
	// contents by asking for names and watching which answer comes back.
	for _, key := range req.Keys {
		if !found[key] {
			result.Denied = append(result.Denied, key)
		}
	}

	if err := s.secrets.MarkUsed(ctx, used); err != nil {
		// Usage tracking is an administrative convenience. Failing it must not
		// fail a delivery that has already been authorized.
		s.logger.Warn("could not record secret usage", "error", err)
	}

	return result, nil
}

func (s *Service) decrypt(ctx context.Context, orgID, projectID, envID uuid.UUID, c *store.SecretSummary) (secretvalue.Value, error) {
	material, err := s.secrets.LoadMaterial(ctx, *c.VersionID)
	if err != nil {
		return secretvalue.Value{}, err
	}

	plaintext, err := s.crypto.Decrypt(ctx, material, crypto.EncryptionContext{
		OrganizationID: orgID,
		ProjectID:      projectID,
		EnvironmentID:  envID,
		SecretID:       c.ID,
		Version:        derefInt(c.Version),
	})
	if err != nil {
		return secretvalue.Value{}, err
	}
	return secretvalue.New(plaintext), nil
}

// ---------------------------------------------------------------------------
// Human reveal
// ---------------------------------------------------------------------------

// Reveal returns a single plaintext value to a human.
//
// This is the only path in the system that sends plaintext toward a browser,
// and it requires READ_SECRET, a capability no role confers. Both the request
// and the result are audited, so an administrator granting themselves
// visibility leaves two records rather than none.
func (s *Service) Reveal(ctx context.Context, rc RequestContext, orgID, projectID, envID, secretID uuid.UUID) (secretvalue.Value, error) {
	summary, err := s.secrets.GetSecret(ctx, orgID, secretID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return secretvalue.Value{}, ErrNotFound
		}
		return secretvalue.Value{}, err
	}

	// The attempt is recorded before the decision, so that a denied reveal is
	// still visible to whoever reviews the trail.
	s.audit.Record(ctx, s.event(rc, audit.EventSecretRevealRequested, audit.OutcomeSuccess, summary,
		DeliverRequest{ProjectID: projectID, EnvironmentID: envID}, "", nil))

	decision, err := s.policy.Authorize(ctx, authz.Request{
		Principal:      rc.Principal,
		Capability:     domain.CapReadSecret,
		OrganizationID: orgID,
		ProjectID:      projectID,
		EnvironmentID:  envID,
		SecretID:       secretID,
		SecretKey:      summary.Key,
	})
	if err != nil {
		return secretvalue.Value{}, err
	}
	if !decision.Allowed {
		s.audit.Record(ctx, s.event(rc, audit.EventSecretRevealed, audit.OutcomeDenied, summary,
			DeliverRequest{ProjectID: projectID, EnvironmentID: envID}, decision.Reason, map[string]any{
				"deny_code": string(decision.Code),
			}))
		return secretvalue.Value{}, ErrNotAuthorized
	}

	if !summary.Deliverable(s.now()) {
		return secretvalue.Value{}, ErrSecretUnavailable
	}

	value, err := s.decrypt(ctx, orgID, projectID, envID, summary)
	if err != nil {
		s.logger.Error("secret material could not be decrypted for reveal",
			"secret_id", secretID.String(), "error", err)
		return secretvalue.Value{}, ErrSecretUnavailable
	}

	s.audit.Record(ctx, s.event(rc, audit.EventSecretRevealed, audit.OutcomeSuccess, summary,
		DeliverRequest{ProjectID: projectID, EnvironmentID: envID}, "", map[string]any{
			"version":  derefInt(summary.Version),
			"grant_id": grantIDString(decision),
		}))

	return value, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// CreateRequest describes a new secret and its first version.
type CreateRequest struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	Key            string
	Description    string
	Value          secretvalue.Value
	ExpiresAt      *time.Time
}

// Create stores a new secret with its first version.
//
// The write and its audit event share one transaction. An encrypted value that
// exists with no record of who created it, or a record of a creation that did
// not happen, would each be worse than the operation failing.
func (s *Service) Create(ctx context.Context, rc RequestContext, req CreateRequest) (*domain.Secret, *domain.SecretVersion, error) {
	created, err := s.CreateMany(ctx, rc, []CreateRequest{req})
	if err != nil {
		return nil, nil, err
	}
	return created[0].Secret, created[0].Version, nil
}

// Created pairs a stored secret with the version that acceptance produced.
type Created struct {
	Secret  *domain.Secret
	Version *domain.SecretVersion
}

// CreateMany stores several secrets in one transaction.
//
// One transaction for the whole batch, not one per secret. Someone pasting a
// .env file is describing a configuration, not twenty independent facts: half
// of it landing because the eleventh key was malformed leaves an environment
// that boots an application with a partial configuration, which is a worse
// failure than none of it landing.
//
// Every request in the batch is authorized on its own. They share a
// transaction, not a permission check.
func (s *Service) CreateMany(ctx context.Context, rc RequestContext, reqs []CreateRequest) ([]Created, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	for _, req := range reqs {
		if req.Value.Len() > MaxSecretBytes {
			return nil, ErrValueTooLarge
		}

		decision, err := s.policy.Authorize(ctx, authz.Request{
			Principal:      rc.Principal,
			Capability:     domain.CapCreateSecret,
			OrganizationID: req.OrganizationID,
			ProjectID:      req.ProjectID,
			EnvironmentID:  req.EnvironmentID,
		})
		if err != nil {
			return nil, err
		}
		if !decision.Allowed {
			return nil, ErrNotAuthorized
		}
	}

	created := make([]Created, len(reqs))

	err := store.InTx(ctx, s.db, func(tx pgx.Tx) error {
		for i, req := range reqs {
			secret, version, err := s.createInTx(ctx, tx, rc, req)
			if err != nil {
				return err
			}
			created[i] = Created{Secret: secret, Version: version}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

// createInTx is the single path that brings a new secret into being.
//
// Both the one-at-a-time and the batch entry points go through here, so there
// is exactly one place where a value is encrypted and one place where the
// creation is recorded. A second implementation of this would be a second
// chance to get the encryption context wrong.
func (s *Service) createInTx(ctx context.Context, tx pgx.Tx, rc RequestContext, req CreateRequest) (*domain.Secret, *domain.SecretVersion, error) {
	createdBy := actorID(rc.Principal)

	secret, err := s.secrets.CreateSecret(ctx, tx, req.EnvironmentID, req.Key, req.Description, createdBy)
	if err != nil {
		return nil, nil, err
	}

	// Encryption happens inside the transaction because the encryption context
	// binds the secret's identifier, which does not exist until the row does.
	enc, err := s.crypto.Encrypt(ctx, req.Value.Expose(), crypto.EncryptionContext{
		OrganizationID: req.OrganizationID,
		ProjectID:      req.ProjectID,
		EnvironmentID:  req.EnvironmentID,
		SecretID:       secret.ID,
		Version:        1,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("secrets: encrypting value: %w", err)
	}

	version, err := s.secrets.CreateVersion(ctx, tx, secret.ID, enc, createdBy, req.ExpiresAt)
	if err != nil {
		return nil, nil, err
	}

	// One event per secret, even in a batch. An audit trail that recorded
	// "twenty secrets created" would not answer "when did STRIPE_KEY appear".
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		OrganizationID: req.OrganizationID,
		Type:           audit.EventSecretCreated,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      rc.Principal.ActorType,
		ActorID:        actorID(rc.Principal),
		ActorLabel:     rc.Principal.DisplayName,
		CredentialID:   credentialID(rc.Principal),
		ProjectID:      &req.ProjectID,
		EnvironmentID:  &req.EnvironmentID,
		SecretID:       &secret.ID,
		SecretKey:      req.Key,
		IPAddress:      rc.ClientIP,
		UserAgent:      rc.UserAgent,
		Metadata: map[string]any{
			"version":            version.Version,
			"encryption_key_id":  enc.KeyID,
			"expires_at_set":     req.ExpiresAt != nil,
			"value_length_bytes": req.Value.Len(),
		},
	}); err != nil {
		return nil, nil, err
	}

	return secret, version, nil
}

// RotateRequest supplies a replacement value for an existing secret.
type RotateRequest struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	SecretID       uuid.UUID
	Value          secretvalue.Value
	ExpiresAt      *time.Time
}

// Rotate stores a new version and makes it active.
//
// Applications keep referring to the same secret; nothing about the reference
// they hold changes. This rotates the value Warder stores. It does not rotate
// the credential at the upstream provider, creating a new database password or
// a new Stripe key remains a separate act, and pretending otherwise would leave
// operators believing a credential had been replaced when it had not.
func (s *Service) Rotate(ctx context.Context, rc RequestContext, req RotateRequest) (*domain.SecretVersion, error) {
	if req.Value.Len() > MaxSecretBytes {
		return nil, ErrValueTooLarge
	}

	summary, err := s.secrets.GetSecret(ctx, req.OrganizationID, req.SecretID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	decision, err := s.policy.Authorize(ctx, authz.Request{
		Principal:      rc.Principal,
		Capability:     domain.CapRotateSecret,
		OrganizationID: req.OrganizationID,
		ProjectID:      req.ProjectID,
		EnvironmentID:  req.EnvironmentID,
		SecretID:       req.SecretID,
		SecretKey:      summary.Key,
	})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, ErrNotAuthorized
	}

	var version *domain.SecretVersion
	err = store.InTx(ctx, s.db, func(tx pgx.Tx) error {
		nextVersion := derefInt(summary.Version) + 1

		enc, err := s.crypto.Encrypt(ctx, req.Value.Expose(), crypto.EncryptionContext{
			OrganizationID: req.OrganizationID,
			ProjectID:      req.ProjectID,
			EnvironmentID:  req.EnvironmentID,
			SecretID:       req.SecretID,
			Version:        nextVersion,
		})
		if err != nil {
			return fmt.Errorf("secrets: encrypting value: %w", err)
		}

		version, err = s.secrets.CreateVersion(ctx, tx, req.SecretID, enc, actorID(rc.Principal), req.ExpiresAt)
		if err != nil {
			return err
		}

		// The version the database assigned must match the one bound into the
		// ciphertext. If a concurrent rotation moved the counter, the binding
		// would be wrong and the value would be undecryptable later, so the
		// transaction is abandoned rather than committing material that cannot
		// be opened.
		if version.Version != nextVersion {
			return fmt.Errorf("secrets: concurrent rotation detected; retry")
		}

		return s.audit.RecordTx(ctx, tx, audit.Event{
			OrganizationID: req.OrganizationID,
			Type:           audit.EventSecretRotated,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      rc.Principal.ActorType,
			ActorID:        actorID(rc.Principal),
			ActorLabel:     rc.Principal.DisplayName,
			CredentialID:   credentialID(rc.Principal),
			ProjectID:      &req.ProjectID,
			EnvironmentID:  &req.EnvironmentID,
			SecretID:       &req.SecretID,
			SecretKey:      summary.Key,
			IPAddress:      rc.ClientIP,
			UserAgent:      rc.UserAgent,
			Metadata: map[string]any{
				"version":           version.Version,
				"previous_version":  derefInt(summary.Version),
				"encryption_key_id": enc.KeyID,
				"upstream_rotated":  false,
			},
		})
	})
	if err != nil {
		return nil, err
	}

	return version, nil
}

// ---------------------------------------------------------------------------
// Audit helpers
// ---------------------------------------------------------------------------

func (s *Service) event(rc RequestContext, t audit.EventType, outcome audit.Outcome, c *store.SecretSummary, req DeliverRequest, reason string, metadata map[string]any) audit.Event {
	ev := audit.Event{
		OrganizationID: rc.Principal.OrganizationID,
		Type:           t,
		Outcome:        outcome,
		ActorType:      rc.Principal.ActorType,
		ActorID:        actorID(rc.Principal),
		ActorLabel:     rc.Principal.DisplayName,
		CredentialID:   credentialID(rc.Principal),
		IPAddress:      rc.ClientIP,
		UserAgent:      rc.UserAgent,
		Reason:         reason,
		Metadata:       metadata,
	}
	if req.ProjectID != uuid.Nil {
		ev.ProjectID = &req.ProjectID
	}
	if req.EnvironmentID != uuid.Nil {
		ev.EnvironmentID = &req.EnvironmentID
	}
	if c != nil {
		ev.SecretID = &c.ID
		ev.SecretKey = c.Key
	}
	return ev
}

func (s *Service) recordDenied(ctx context.Context, rc RequestContext, c *store.SecretSummary, req DeliverRequest, d authz.Decision) {
	s.audit.Record(ctx, s.event(rc, audit.EventAccessDenied, audit.OutcomeDenied, c, req, d.Reason, map[string]any{
		"deny_code":  string(d.Code),
		"capability": string(domain.CapUseSecret),
	}))
}

func unavailableReason(c *store.SecretSummary, now time.Time) string {
	switch {
	case c.VersionID == nil:
		return "secret has no active version"
	case c.VersionStatus != nil && *c.VersionStatus == domain.VersionRevoked:
		return "active version has been revoked"
	case c.Expired(now):
		return "active version has expired"
	default:
		return "secret is not available"
	}
}

func actorID(p domain.Principal) *uuid.UUID {
	if p.ID == uuid.Nil {
		return nil
	}
	id := p.ID
	return &id
}

func credentialID(p domain.Principal) *uuid.UUID {
	if p.CredentialID == uuid.Nil {
		return nil
	}
	id := p.CredentialID
	return &id
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func grantIDString(d authz.Decision) string {
	if d.GrantID == nil {
		return ""
	}
	return d.GrantID.String()
}
