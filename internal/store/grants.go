package store

import (
	"context"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

// GrantRepo covers access grants. It implements authz.GrantSource, which is the
// only way the policy engine learns what an identity may do.
type GrantRepo struct{ db *DB }

// NewGrantRepo constructs the repository.
func NewGrantRepo(db *DB) *GrantRepo { return &GrantRepo{db: db} }

// GrantsForSubject returns every grant held by a subject in an organization.
//
// Expired and revoked grants are filtered in SQL as well as in the policy
// engine. The engine's check is the one that matters: it uses a single
// consistent clock for the whole decision, but excluding them here keeps a
// long-lived identity's grant list from growing into a scan.
func (r *GrantRepo) GrantsForSubject(ctx context.Context, orgID uuid.UUID, subjectType domain.SubjectType, subjectID uuid.UUID) ([]domain.AccessGrant, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, project_id, environment_id, secret_id, capabilities,
		       created_at, expires_at, revoked_at, reason
		FROM access_grants
		WHERE organization_id = $1 AND subject_type = $2 AND subject_id = $3
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())`,
		orgID, string(subjectType), subjectID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []domain.AccessGrant
	for rows.Next() {
		g := domain.AccessGrant{
			OrganizationID: orgID,
			SubjectType:    subjectType,
			SubjectID:      subjectID,
		}
		var caps []string
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.EnvironmentID, &g.SecretID, &caps,
			&g.CreatedAt, &g.ExpiresAt, &g.RevokedAt, &g.Reason); err != nil {
			return nil, translate(err)
		}
		g.Capabilities = capabilitiesFrom(caps)
		out = append(out, g)
	}
	return out, translate(rows.Err())
}

// Create inserts a grant.
func (r *GrantRepo) Create(ctx context.Context, q Queryer, g *domain.AccessGrant) error {
	if q == nil {
		q = r.db.Pool
	}
	return translate(q.QueryRow(ctx, `
		INSERT INTO access_grants
			(organization_id, subject_type, subject_id, project_id, environment_id, secret_id,
			 capabilities, created_by, expires_at, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		g.OrganizationID, string(g.SubjectType), g.SubjectID,
		g.ProjectID, g.EnvironmentID, g.SecretID,
		capabilityStrings(g.Capabilities), nullableUUID(g.CreatedBy), g.ExpiresAt, g.Reason,
	).Scan(&g.ID, &g.CreatedAt))
}

// GrantView is a grant joined with a readable description of its subject, which
// is what the access screen renders.
type GrantView struct {
	domain.AccessGrant
	SubjectName string
	SubjectKind string // the actor type for machines, the role for users
}

// ListForProject returns the grants that touch a project, including the
// organization-wide grants that reach it.
//
// This is the query behind "who can use this secret, and who can see it". It
// deliberately returns both, so the two answers come from one place.
func (r *GrantRepo) ListForProject(ctx context.Context, orgID, projectID uuid.UUID) ([]GrantView, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT g.id, g.subject_type, g.subject_id, g.project_id, g.environment_id, g.secret_id,
		       g.capabilities, g.created_at, g.expires_at, g.revoked_at, g.reason,
		       COALESCE(u.name, m.name, '') AS subject_name,
		       COALESCE(mem.role, m.actor_type, '') AS subject_kind
		FROM access_grants g
		LEFT JOIN users u ON g.subject_type = 'USER' AND u.id = g.subject_id
		LEFT JOIN memberships mem ON g.subject_type = 'USER'
		     AND mem.user_id = g.subject_id AND mem.organization_id = g.organization_id
		LEFT JOIN machine_identities m ON g.subject_type = 'MACHINE' AND m.id = g.subject_id
		WHERE g.organization_id = $1
		  AND (g.project_id = $2 OR g.project_id IS NULL)
		  AND g.revoked_at IS NULL
		ORDER BY g.created_at DESC`, orgID, projectID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []GrantView
	for rows.Next() {
		v := GrantView{}
		v.OrganizationID = orgID
		var caps []string
		var subjectType string
		if err := rows.Scan(&v.ID, &subjectType, &v.SubjectID, &v.ProjectID, &v.EnvironmentID, &v.SecretID,
			&caps, &v.CreatedAt, &v.ExpiresAt, &v.RevokedAt, &v.Reason,
			&v.SubjectName, &v.SubjectKind); err != nil {
			return nil, translate(err)
		}
		v.SubjectType = domain.SubjectType(subjectType)
		v.Capabilities = capabilitiesFrom(caps)
		out = append(out, v)
	}
	return out, translate(rows.Err())
}

// Revoke ends a grant immediately.
func (r *GrantRepo) Revoke(ctx context.Context, q Queryer, orgID, grantID uuid.UUID) error {
	if q == nil {
		q = r.db.Pool
	}
	tag, err := q.Exec(ctx, `
		UPDATE access_grants SET revoked_at = now()
		WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL`, grantID, orgID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Get loads a single grant.
func (r *GrantRepo) Get(ctx context.Context, orgID, grantID uuid.UUID) (*domain.AccessGrant, error) {
	g := &domain.AccessGrant{ID: grantID, OrganizationID: orgID}
	var caps []string
	var subjectType string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT subject_type, subject_id, project_id, environment_id, secret_id,
		       capabilities, created_at, expires_at, revoked_at, reason
		FROM access_grants WHERE id = $1 AND organization_id = $2`, grantID, orgID,
	).Scan(&subjectType, &g.SubjectID, &g.ProjectID, &g.EnvironmentID, &g.SecretID,
		&caps, &g.CreatedAt, &g.ExpiresAt, &g.RevokedAt, &g.Reason)
	if err != nil {
		return nil, translate(err)
	}
	g.SubjectType = domain.SubjectType(subjectType)
	g.Capabilities = capabilitiesFrom(caps)
	return g, nil
}
