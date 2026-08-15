package store

import (
	"context"
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MachineRepo covers machine identities, their long-lived tokens, and the
// short-lived runtime sessions minted from those tokens.
type MachineRepo struct{ db *DB }

// NewMachineRepo constructs the repository.
func NewMachineRepo(db *DB) *MachineRepo { return &MachineRepo{db: db} }

// CreateIdentity inserts a machine identity.
func (r *MachineRepo) CreateIdentity(ctx context.Context, q Queryer, m *domain.MachineIdentity) error {
	if q == nil {
		q = r.db.Pool
	}
	return translate(q.QueryRow(ctx, `
		INSERT INTO machine_identities (organization_id, name, actor_type, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		m.OrganizationID, m.Name, string(m.ActorType), nullableUUID(m.CreatedBy), m.ExpiresAt,
	).Scan(&m.ID, &m.CreatedAt))
}

// GetIdentity loads a machine identity scoped to its organization.
func (r *MachineRepo) GetIdentity(ctx context.Context, orgID, id uuid.UUID) (*domain.MachineIdentity, error) {
	m := &domain.MachineIdentity{ID: id, OrganizationID: orgID}
	var actorType string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT name, actor_type, created_at, disabled_at, expires_at
		FROM machine_identities WHERE id = $1 AND organization_id = $2`, id, orgID,
	).Scan(&m.Name, &actorType, &m.CreatedAt, &m.DisabledAt, &m.ExpiresAt)
	if err != nil {
		return nil, translate(err)
	}
	m.ActorType = domain.ActorType(actorType)
	return m, nil
}

// ListIdentities returns the machine identities in an organization.
func (r *MachineRepo) ListIdentities(ctx context.Context, orgID uuid.UUID) ([]domain.MachineIdentity, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, actor_type, created_at, disabled_at, expires_at
		FROM machine_identities WHERE organization_id = $1 ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []domain.MachineIdentity
	for rows.Next() {
		m := domain.MachineIdentity{OrganizationID: orgID}
		var actorType string
		if err := rows.Scan(&m.ID, &m.Name, &actorType, &m.CreatedAt, &m.DisabledAt, &m.ExpiresAt); err != nil {
			return nil, translate(err)
		}
		m.ActorType = domain.ActorType(actorType)
		out = append(out, m)
	}
	return out, translate(rows.Err())
}

// DisableIdentity stops an identity from authenticating, along with every
// short-lived session already derived from its tokens.
//
// Its long-lived tokens stop working immediately without being touched, because
// token verification re-reads the identity on every request. Runtime sessions
// do not: they are verified against their own record, so a session minted a
// minute ago would keep delivering secrets until it expired. Disabling an
// identity has to mean the next request is denied, not the next request after
// the session lapses, so they are revoked here, in the same transaction.
func (r *MachineRepo) DisableIdentity(ctx context.Context, orgID, id uuid.UUID) error {
	return InTx(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE machine_identities SET disabled_at = now()
			WHERE id = $1 AND organization_id = $2 AND disabled_at IS NULL`, id, orgID)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}

		_, err = tx.Exec(ctx, `
			UPDATE runtime_sessions SET revoked_at = now()
			WHERE revoked_at IS NULL
			  AND organization_id = $2
			  AND source_credential_id IN (
			      SELECT id FROM machine_tokens WHERE machine_identity_id = $1
			  )`, id, orgID)
		return translate(err)
	})
}

// ---------------------------------------------------------------------------
// Machine tokens
// ---------------------------------------------------------------------------

// MachineToken is a scoped, long-lived runtime credential.
type MachineToken struct {
	ID                uuid.UUID
	MachineIdentityID uuid.UUID
	OrganizationID    uuid.UUID
	Name              string
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	Capabilities      []domain.Capability
	SecretKeys        []string
	PublicID          string
	CreatedAt         time.Time
	ExpiresAt         *time.Time
	RevokedAt         *time.Time
	LastUsedAt        *time.Time
}

// Active reports whether the token may still be used.
func (t *MachineToken) Active(now time.Time) bool {
	if t.RevokedAt != nil && !t.RevokedAt.After(now) {
		return false
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(now) {
		return false
	}
	return true
}

// CreateToken stores a minted token, keeping only its verifier.
func (r *MachineRepo) CreateToken(ctx context.Context, q Queryer, t *MachineToken, hash []byte, createdBy *uuid.UUID) error {
	if q == nil {
		q = r.db.Pool
	}
	return translate(q.QueryRow(ctx, `
		INSERT INTO machine_tokens
			(machine_identity_id, organization_id, name, project_id, environment_id,
			 capabilities, secret_keys, token_hash, token_prefix, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at`,
		t.MachineIdentityID, t.OrganizationID, t.Name, t.ProjectID, t.EnvironmentID,
		capabilityStrings(t.Capabilities), textArray(t.SecretKeys), hash, t.PublicID, createdBy, t.ExpiresAt,
	).Scan(&t.ID, &t.CreatedAt))
}

// TokenCandidate is a stored token together with the identity behind it, loaded
// as one row so that authentication does not need a second round trip.
type TokenCandidate struct {
	MachineToken
	TokenHash []byte

	IdentityName       string
	IdentityActorType  domain.ActorType
	IdentityDisabledAt *time.Time
	IdentityExpiresAt  *time.Time
}

// FindTokenByPublicID loads the candidate row for a presented credential.
// Verification of the secret half happens in the caller, in constant time.
func (r *MachineRepo) FindTokenByPublicID(ctx context.Context, publicID string) (*TokenCandidate, error) {
	c := &TokenCandidate{}
	var caps []string
	var actorType string

	err := r.db.Pool.QueryRow(ctx, `
		SELECT t.id, t.machine_identity_id, t.organization_id, t.name,
		       t.project_id, t.environment_id, t.capabilities, t.secret_keys,
		       t.token_hash, t.token_prefix, t.created_at, t.expires_at, t.revoked_at, t.last_used_at,
		       i.name, i.actor_type, i.disabled_at, i.expires_at
		FROM machine_tokens t
		JOIN machine_identities i ON i.id = t.machine_identity_id
		WHERE t.token_prefix = $1`, publicID,
	).Scan(&c.ID, &c.MachineIdentityID, &c.OrganizationID, &c.Name,
		&c.ProjectID, &c.EnvironmentID, &caps, &c.SecretKeys,
		&c.TokenHash, &c.PublicID, &c.CreatedAt, &c.ExpiresAt, &c.RevokedAt, &c.LastUsedAt,
		&c.IdentityName, &actorType, &c.IdentityDisabledAt, &c.IdentityExpiresAt)
	if err != nil {
		return nil, translate(err)
	}

	c.Capabilities = capabilitiesFrom(caps)
	c.IdentityActorType = domain.ActorType(actorType)
	return c, nil
}

// ListTokens returns the tokens scoped to a project, without their verifiers.
func (r *MachineRepo) ListTokens(ctx context.Context, orgID, projectID uuid.UUID) ([]TokenCandidate, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT t.id, t.machine_identity_id, t.organization_id, t.name,
		       t.project_id, t.environment_id, t.capabilities, t.secret_keys,
		       t.token_prefix, t.created_at, t.expires_at, t.revoked_at, t.last_used_at,
		       i.name, i.actor_type
		FROM machine_tokens t
		JOIN machine_identities i ON i.id = t.machine_identity_id
		WHERE t.organization_id = $1 AND t.project_id = $2
		ORDER BY t.created_at DESC`, orgID, projectID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []TokenCandidate
	for rows.Next() {
		var c TokenCandidate
		var caps []string
		var actorType string
		if err := rows.Scan(&c.ID, &c.MachineIdentityID, &c.OrganizationID, &c.Name,
			&c.ProjectID, &c.EnvironmentID, &caps, &c.SecretKeys,
			&c.PublicID, &c.CreatedAt, &c.ExpiresAt, &c.RevokedAt, &c.LastUsedAt,
			&c.IdentityName, &actorType); err != nil {
			return nil, translate(err)
		}
		c.Capabilities = capabilitiesFrom(caps)
		c.IdentityActorType = domain.ActorType(actorType)
		out = append(out, c)
	}
	return out, translate(rows.Err())
}

// RevokeToken revokes a token and, in the same statement batch, every runtime
// session derived from it.
//
// Revoking only the long-lived token would leave a window of up to the runtime
// session lifetime in which secret retrieval still succeeds. "What happens if I
// revoke access?" has to answer "the next request is denied".
func (r *MachineRepo) RevokeToken(ctx context.Context, orgID, tokenID uuid.UUID) error {
	return InTx(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE machine_tokens SET revoked_at = now()
			WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL`, tokenID, orgID)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}

		_, err = tx.Exec(ctx, `
			UPDATE runtime_sessions SET revoked_at = now()
			WHERE source_credential_id = $1 AND revoked_at IS NULL`, tokenID)
		return translate(err)
	})
}

// TouchToken records that a token was used.
func (r *MachineRepo) TouchToken(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE machine_tokens SET last_used_at = now() WHERE id = $1`, id)
	return translate(err)
}

// ---------------------------------------------------------------------------
// Runtime sessions
// ---------------------------------------------------------------------------

// RuntimeSession is the short-lived credential presented to the secret
// delivery endpoint.
type RuntimeSession struct {
	ID                 uuid.UUID
	OrganizationID     uuid.UUID
	SubjectType        domain.SubjectType
	SubjectID          uuid.UUID
	ActorType          domain.ActorType
	ProjectID          uuid.UUID
	EnvironmentID      uuid.UUID
	Capabilities       []domain.Capability
	SecretKeys         []string
	SourceCredentialID *uuid.UUID
	PublicID           string
	CreatedAt          time.Time
	ExpiresAt          time.Time
	RevokedAt          *time.Time
}

// CreateRuntimeSession stores a minted runtime session.
func (r *MachineRepo) CreateRuntimeSession(ctx context.Context, s *RuntimeSession, hash []byte) error {
	return translate(r.db.Pool.QueryRow(ctx, `
		INSERT INTO runtime_sessions
			(organization_id, subject_type, subject_id, actor_type, project_id, environment_id,
			 capabilities, secret_keys, source_credential_id, token_hash, token_prefix, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at`,
		s.OrganizationID, string(s.SubjectType), s.SubjectID, string(s.ActorType),
		s.ProjectID, s.EnvironmentID, capabilityStrings(s.Capabilities), textArray(s.SecretKeys),
		s.SourceCredentialID, hash, s.PublicID, s.ExpiresAt,
	).Scan(&s.ID, &s.CreatedAt))
}

// RuntimeSessionCandidate is a stored runtime session and its verifier.
type RuntimeSessionCandidate struct {
	RuntimeSession
	TokenHash []byte
}

// FindRuntimeSessionByPublicID loads the candidate row for a presented runtime
// credential.
func (r *MachineRepo) FindRuntimeSessionByPublicID(ctx context.Context, publicID string) (*RuntimeSessionCandidate, error) {
	c := &RuntimeSessionCandidate{}
	var caps []string
	var subjectType, actorType string

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, organization_id, subject_type, subject_id, actor_type,
		       project_id, environment_id, capabilities, secret_keys,
		       source_credential_id, token_hash, token_prefix, created_at, expires_at, revoked_at
		FROM runtime_sessions WHERE token_prefix = $1`, publicID,
	).Scan(&c.ID, &c.OrganizationID, &subjectType, &c.SubjectID, &actorType,
		&c.ProjectID, &c.EnvironmentID, &caps, &c.SecretKeys,
		&c.SourceCredentialID, &c.TokenHash, &c.PublicID, &c.CreatedAt, &c.ExpiresAt, &c.RevokedAt)
	if err != nil {
		return nil, translate(err)
	}

	c.Capabilities = capabilitiesFrom(caps)
	c.SubjectType = domain.SubjectType(subjectType)
	c.ActorType = domain.ActorType(actorType)
	return c, nil
}

// PurgeExpiredRuntimeSessions removes sessions that can no longer authenticate.
// They are already refused on presentation; this keeps the table from growing
// without bound.
func (r *MachineRepo) PurgeExpiredRuntimeSessions(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := r.db.Pool.Exec(ctx,
		`DELETE FROM runtime_sessions WHERE expires_at < now() - $1::interval`,
		olderThan.String())
	if err != nil {
		return 0, translate(err)
	}
	return tag.RowsAffected(), nil
}

func nullableUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
