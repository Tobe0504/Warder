package store

import (
	"context"
	"strings"
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

// AccountRepo covers organizations, users, memberships, and login sessions.
type AccountRepo struct{ db *DB }

// NewAccountRepo constructs the repository.
func NewAccountRepo(db *DB) *AccountRepo { return &AccountRepo{db: db} }

// CreateOrganization inserts an organization.
func (r *AccountRepo) CreateOrganization(ctx context.Context, q Queryer, name, slug string) (*domain.Organization, error) {
	if q == nil {
		q = r.db.Pool
	}
	org := &domain.Organization{Name: name, Slug: slug}
	err := q.QueryRow(ctx, `
		INSERT INTO organizations (name, slug) VALUES ($1, $2)
		RETURNING id, created_at, updated_at`,
		name, slug,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return org, nil
}

// GetOrganization loads an organization by id.
func (r *AccountRepo) GetOrganization(ctx context.Context, id uuid.UUID) (*domain.Organization, error) {
	org := &domain.Organization{ID: id}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT name, slug, created_at, updated_at FROM organizations WHERE id = $1`, id,
	).Scan(&org.Name, &org.Slug, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return org, nil
}

// CreateUser inserts a user with an already-hashed password.
func (r *AccountRepo) CreateUser(ctx context.Context, q Queryer, email, name, passwordHash string) (*domain.User, error) {
	if q == nil {
		q = r.db.Pool
	}
	// Addresses are stored normalized so that lookup and the uniqueness index
	// agree without depending on the caller.
	email = strings.ToLower(strings.TrimSpace(email))

	user := &domain.User{Email: email, Name: name}
	err := q.QueryRow(ctx, `
		INSERT INTO users (email, name, password_hash) VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		email, name, passwordHash,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return user, nil
}

// GetUserByEmail loads a user for authentication. It is the only path that
// selects the password hash.
func (r *AccountRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	user := &domain.User{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, created_at, updated_at, disabled_at
		FROM users WHERE lower(email) = $1`, email,
	).Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash,
		&user.CreatedAt, &user.UpdatedAt, &user.DisabledAt)
	if err != nil {
		return nil, translate(err)
	}
	return user, nil
}

// GetUser loads a user by id without the password hash.
func (r *AccountRepo) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user := &domain.User{ID: id}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT email, name, created_at, updated_at, disabled_at FROM users WHERE id = $1`, id,
	).Scan(&user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt, &user.DisabledAt)
	if err != nil {
		return nil, translate(err)
	}
	return user, nil
}

// UpdatePasswordHash replaces a stored hash, used to upgrade Argon2 parameters
// after a successful login.
func (r *AccountRepo) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, hash)
	return translate(err)
}

// CreateMembership binds a user to an organization.
func (r *AccountRepo) CreateMembership(ctx context.Context, q Queryer, m *domain.Membership) (*domain.Membership, error) {
	if q == nil {
		q = r.db.Pool
	}
	err := q.QueryRow(ctx, `
		INSERT INTO memberships (organization_id, user_id, role, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		m.OrganizationID, m.UserID, string(m.Role), m.CreatedBy, m.ExpiresAt,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return m, nil
}

// GetMembership loads a user's membership in an organization.
func (r *AccountRepo) GetMembership(ctx context.Context, orgID, userID uuid.UUID) (*domain.Membership, error) {
	m := &domain.Membership{OrganizationID: orgID, UserID: userID}
	var role string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, role, created_at, expires_at, revoked_at
		FROM memberships WHERE organization_id = $1 AND user_id = $2`,
		orgID, userID,
	).Scan(&m.ID, &role, &m.CreatedAt, &m.ExpiresAt, &m.RevokedAt)
	if err != nil {
		return nil, translate(err)
	}
	m.Role = domain.Role(role)
	return m, nil
}

// FindPrimaryMembership returns the organization a user belongs to. The MVP
// assumes a single organization per user; the query is ordered so the result is
// deterministic if that assumption is relaxed later.
func (r *AccountRepo) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*domain.Membership, error) {
	m := &domain.Membership{UserID: userID}
	var role string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, organization_id, role, created_at, expires_at, revoked_at
		FROM memberships
		WHERE user_id = $1 AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at ASC LIMIT 1`,
		userID,
	).Scan(&m.ID, &m.OrganizationID, &role, &m.CreatedAt, &m.ExpiresAt, &m.RevokedAt)
	if err != nil {
		return nil, translate(err)
	}
	m.Role = domain.Role(role)
	return m, nil
}

// ListMembers returns the members of an organization with their user records.
func (r *AccountRepo) ListMembers(ctx context.Context, orgID uuid.UUID) ([]Member, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT m.id, m.role, m.created_at, m.expires_at, m.revoked_at,
		       u.id, u.email, u.name
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.organization_id = $1
		ORDER BY m.created_at ASC`, orgID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var m Member
		var role string
		if err := rows.Scan(&m.MembershipID, &role, &m.CreatedAt, &m.ExpiresAt, &m.RevokedAt,
			&m.UserID, &m.Email, &m.Name); err != nil {
			return nil, translate(err)
		}
		m.Role = domain.Role(role)
		members = append(members, m)
	}
	return members, translate(rows.Err())
}

// Member is an organization member joined with their user record.
type Member struct {
	MembershipID uuid.UUID
	UserID       uuid.UUID
	Email        string
	Name         string
	Role         domain.Role
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	RevokedAt    *time.Time
}

// RevokeMembership ends a membership immediately.
func (r *AccountRepo) RevokeMembership(ctx context.Context, orgID, membershipID uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE memberships SET revoked_at = now()
		WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL`,
		membershipID, orgID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// Session is a browser or CLI login session.
type Session struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Kind           string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
	LastUsedAt     *time.Time
}

// CreateSession stores a session, keyed by the credential's public handle with
// only the verifier retained.
func (r *AccountRepo) CreateSession(ctx context.Context, s *Session, publicID string, hash []byte, ip, userAgent string) error {
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO user_sessions
			(user_id, organization_id, kind, token_hash, token_prefix, expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		s.UserID, s.OrganizationID, s.Kind, hash, publicID, s.ExpiresAt, nullInet(ip), userAgent,
	).Scan(&s.ID, &s.CreatedAt)
	return translate(err)
}

// SessionCandidate is the stored record for a presented session credential.
type SessionCandidate struct {
	Session
	TokenHash []byte
}

// FindSessionByPublicID loads the single candidate row for a presented
// credential. Verification of the secret half happens in the caller, in
// constant time; this lookup is by the public handle only.
func (r *AccountRepo) FindSessionByPublicID(ctx context.Context, publicID string) (*SessionCandidate, error) {
	c := &SessionCandidate{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, user_id, organization_id, kind, token_hash, created_at, expires_at, revoked_at, last_used_at
		FROM user_sessions WHERE token_prefix = $1`, publicID,
	).Scan(&c.ID, &c.UserID, &c.OrganizationID, &c.Kind, &c.TokenHash,
		&c.CreatedAt, &c.ExpiresAt, &c.RevokedAt, &c.LastUsedAt)
	if err != nil {
		return nil, translate(err)
	}
	return c, nil
}

// TouchSession records that a session was used.
func (r *AccountRepo) TouchSession(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE user_sessions SET last_used_at = now() WHERE id = $1`, id)
	return translate(err)
}

// RevokeSession ends a session immediately.
func (r *AccountRepo) RevokeSession(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	return translate(err)
}

// RevokeSessionsForUser ends every session a user holds, which is what a
// password change or an offboarding needs.
func (r *AccountRepo) RevokeSessionsForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return translate(err)
}
