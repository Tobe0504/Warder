package store

import (
	"context"
	"time"

	"github.com/Tobe0504/Warder/internal/crypto"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SecretRepo covers secret metadata and encrypted material.
//
// The split matters: every method here except LoadMaterial reads from public
// only and cannot return ciphertext. Listing secrets, rendering the dashboard,
// and resolving which keys a runtime asked for all happen without the
// secret_material schema being touched, so authorization runs to completion
// before any encrypted material is loaded, let alone decrypted.
type SecretRepo struct{ db *DB }

// NewSecretRepo constructs the repository.
func NewSecretRepo(db *DB) *SecretRepo { return &SecretRepo{db: db} }

// SecretSummary is the metadata view of a secret: everything the dashboard
// needs and nothing that could become a value.
type SecretSummary struct {
	ID            uuid.UUID
	EnvironmentID uuid.UUID
	Key           string
	Description   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastUsedAt    *time.Time

	// Active version metadata, absent when a secret has no deliverable version.
	VersionID       *uuid.UUID
	Version         *int
	VersionStatus   *domain.VersionStatus
	VersionExpires  *time.Time
	VersionRevoked  *time.Time
	EncryptionKeyID *string
}

// Expired reports whether the active version has passed its expiry.
func (s *SecretSummary) Expired(now time.Time) bool {
	return s.VersionExpires != nil && !s.VersionExpires.After(now)
}

// Deliverable reports whether this secret currently has a version that may be
// released to a runtime.
func (s *SecretSummary) Deliverable(now time.Time) bool {
	if s.VersionID == nil || s.VersionStatus == nil {
		return false
	}
	if *s.VersionStatus != domain.VersionActive {
		return false
	}
	if s.VersionRevoked != nil && !s.VersionRevoked.After(now) {
		return false
	}
	return !s.Expired(now)
}

const secretSummaryColumns = `
	s.id, s.environment_id, s.key, s.description, s.created_at, s.updated_at, s.last_used_at,
	v.id, v.version, v.status, v.expires_at, v.revoked_at, v.encryption_key_id`

func scanSecretSummary(row pgx.Row) (*SecretSummary, error) {
	var s SecretSummary
	var status *string
	err := row.Scan(&s.ID, &s.EnvironmentID, &s.Key, &s.Description, &s.CreatedAt, &s.UpdatedAt, &s.LastUsedAt,
		&s.VersionID, &s.Version, &status, &s.VersionExpires, &s.VersionRevoked, &s.EncryptionKeyID)
	if err != nil {
		return nil, translate(err)
	}
	if status != nil {
		vs := domain.VersionStatus(*status)
		s.VersionStatus = &vs
	}
	return &s, nil
}

// CreateSecret inserts secret metadata. The value arrives separately, as a
// version, so that this table never receives one.
func (r *SecretRepo) CreateSecret(ctx context.Context, q Queryer, envID uuid.UUID, key, description string, createdBy *uuid.UUID) (*domain.Secret, error) {
	if q == nil {
		q = r.db.Pool
	}
	s := &domain.Secret{EnvironmentID: envID, Key: key, Description: description}
	err := q.QueryRow(ctx, `
		INSERT INTO secrets (environment_id, key, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		envID, key, description, createdBy,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return s, nil
}

// ListSecrets returns metadata for every secret in an environment, joined to
// its active version. No ciphertext is read.
func (r *SecretRepo) ListSecrets(ctx context.Context, orgID, envID uuid.UUID) ([]SecretSummary, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+secretSummaryColumns+`
		FROM secrets s
		JOIN environments e ON e.id = s.environment_id
		JOIN projects p ON p.id = e.project_id
		LEFT JOIN secret_versions v ON v.secret_id = s.id AND v.status = 'ACTIVE'
		WHERE s.environment_id = $1 AND p.organization_id = $2 AND s.deleted_at IS NULL
		ORDER BY s.key ASC`, envID, orgID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []SecretSummary
	for rows.Next() {
		s, err := scanSecretSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, translate(rows.Err())
}

// GetSecret loads one secret's metadata, scoped to the organization.
func (r *SecretRepo) GetSecret(ctx context.Context, orgID, secretID uuid.UUID) (*SecretSummary, error) {
	return scanSecretSummary(r.db.Pool.QueryRow(ctx, `
		SELECT `+secretSummaryColumns+`
		FROM secrets s
		JOIN environments e ON e.id = s.environment_id
		JOIN projects p ON p.id = e.project_id
		LEFT JOIN secret_versions v ON v.secret_id = s.id AND v.status = 'ACTIVE'
		WHERE s.id = $1 AND p.organization_id = $2 AND s.deleted_at IS NULL`, secretID, orgID))
}

// ResolveForDelivery loads metadata for the specific keys a runtime asked for.
//
// This is the first step of secret delivery and it returns no ciphertext by
// design: the caller authorizes each key against the policy engine using these
// rows, and only then asks for material. Passing an empty keys slice returns
// every secret in the environment, which is what a runtime that did not narrow
// its request receives.
func (r *SecretRepo) ResolveForDelivery(ctx context.Context, envID uuid.UUID, keys []string) ([]SecretSummary, error) {
	query := `
		SELECT ` + secretSummaryColumns + `
		FROM secrets s
		LEFT JOIN secret_versions v ON v.secret_id = s.id AND v.status = 'ACTIVE'
		WHERE s.environment_id = $1 AND s.deleted_at IS NULL`
	args := []any{envID}

	if len(keys) > 0 {
		query += ` AND s.key = ANY($2)`
		args = append(args, keys)
	}
	query += ` ORDER BY s.key ASC`

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []SecretSummary
	for rows.Next() {
		s, err := scanSecretSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, translate(rows.Err())
}

// LoadMaterial reads the encrypted material for one version.
//
// This is the only method in the package that reads from the secret_material
// schema. Callers must have completed an authorization decision before calling
// it; see secrets.Service.Deliver, which is the only caller in the application.
func (r *SecretRepo) LoadMaterial(ctx context.Context, versionID uuid.UUID) (*crypto.EncryptedSecret, error) {
	enc := &crypto.EncryptedSecret{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT scheme, algorithm, key_id, wrapped_data_key, nonce, ciphertext
		FROM secret_material.secret_version_material
		WHERE secret_version_id = $1`, versionID,
	).Scan(&enc.Scheme, &enc.Algorithm, &enc.KeyID, &enc.WrappedDataKey, &enc.Nonce, &enc.Ciphertext)
	if err != nil {
		return nil, translate(err)
	}
	return enc, nil
}

// CreateVersion writes a new version and its material, superseding whatever was
// active.
//
// Both writes happen in one transaction with the supersede, so a rotation
// cannot leave a secret with no active version, or with material missing for
// the version that is now serving traffic.
func (r *SecretRepo) CreateVersion(ctx context.Context, tx pgx.Tx, secretID uuid.UUID, enc *crypto.EncryptedSecret, createdBy *uuid.UUID, expiresAt *time.Time) (*domain.SecretVersion, error) {
	// Retire the current active version first; the partial unique index refuses
	// a second active row.
	if _, err := tx.Exec(ctx, `
		UPDATE secret_versions SET status = 'SUPERSEDED'
		WHERE secret_id = $1 AND status = 'ACTIVE'`, secretID); err != nil {
		return nil, translate(err)
	}

	v := &domain.SecretVersion{
		SecretID:        secretID,
		Status:          domain.VersionActive,
		ExpiresAt:       expiresAt,
		EncryptionKeyID: enc.KeyID,
	}

	err := tx.QueryRow(ctx, `
		INSERT INTO secret_versions (secret_id, version, status, created_by, expires_at, encryption_key_id)
		VALUES (
			$1,
			COALESCE((SELECT max(version) FROM secret_versions WHERE secret_id = $1), 0) + 1,
			'ACTIVE', $2, $3, $4
		)
		RETURNING id, version, created_at`,
		secretID, createdBy, expiresAt, enc.KeyID,
	).Scan(&v.ID, &v.Version, &v.CreatedAt)
	if err != nil {
		return nil, translate(err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO secret_material.secret_version_material
			(secret_version_id, scheme, algorithm, key_id, wrapped_data_key, nonce, ciphertext)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		v.ID, enc.Scheme, enc.Algorithm, enc.KeyID, enc.WrappedDataKey, enc.Nonce, enc.Ciphertext,
	); err != nil {
		return nil, translate(err)
	}

	if _, err := tx.Exec(ctx, `UPDATE secrets SET updated_at = now() WHERE id = $1`, secretID); err != nil {
		return nil, translate(err)
	}

	return v, nil
}

// ListVersions returns version metadata, never material.
func (r *SecretRepo) ListVersions(ctx context.Context, orgID, secretID uuid.UUID) ([]domain.SecretVersion, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT v.id, v.version, v.status, v.created_at, v.created_by, v.expires_at, v.revoked_at, v.encryption_key_id
		FROM secret_versions v
		JOIN secrets s ON s.id = v.secret_id
		JOIN environments e ON e.id = s.environment_id
		JOIN projects p ON p.id = e.project_id
		WHERE v.secret_id = $1 AND p.organization_id = $2
		ORDER BY v.version DESC`, secretID, orgID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []domain.SecretVersion
	for rows.Next() {
		v := domain.SecretVersion{SecretID: secretID}
		var status string
		var createdBy *uuid.UUID
		if err := rows.Scan(&v.ID, &v.Version, &status, &v.CreatedAt, &createdBy,
			&v.ExpiresAt, &v.RevokedAt, &v.EncryptionKeyID); err != nil {
			return nil, translate(err)
		}
		v.Status = domain.VersionStatus(status)
		if createdBy != nil {
			v.CreatedBy = *createdBy
		}
		out = append(out, v)
	}
	return out, translate(rows.Err())
}

// RevokeVersion marks a version as never deliverable again.
func (r *SecretRepo) RevokeVersion(ctx context.Context, q Queryer, secretID, versionID uuid.UUID) error {
	if q == nil {
		q = r.db.Pool
	}
	tag, err := q.Exec(ctx, `
		UPDATE secret_versions SET status = 'REVOKED', revoked_at = now()
		WHERE id = $1 AND secret_id = $2 AND revoked_at IS NULL`, versionID, secretID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ActivateVersion rolls back to an earlier version, which is the recovery path
// when a rotation turns out to have stored the wrong value.
//
// A revoked version stays revoked: revocation is a statement that the
// underlying credential is no longer trustworthy, and rollback must not quietly
// undo it.
func (r *SecretRepo) ActivateVersion(ctx context.Context, tx pgx.Tx, secretID, versionID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE secret_versions SET status = 'SUPERSEDED'
		WHERE secret_id = $1 AND status = 'ACTIVE'`, secretID); err != nil {
		return translate(err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE secret_versions SET status = 'ACTIVE'
		WHERE id = $1 AND secret_id = $2 AND revoked_at IS NULL AND status <> 'REVOKED'`,
		versionID, secretID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetVersionExpiry sets or clears an expiry on a version.
func (r *SecretRepo) SetVersionExpiry(ctx context.Context, q Queryer, secretID, versionID uuid.UUID, expiresAt *time.Time) error {
	if q == nil {
		q = r.db.Pool
	}
	tag, err := q.Exec(ctx, `
		UPDATE secret_versions SET expires_at = $3 WHERE id = $1 AND secret_id = $2`,
		versionID, secretID, expiresAt)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkUsed records that secrets were delivered, so an administrator can see
// whether anything still depends on a credential before revoking it.
func (r *SecretRepo) MarkUsed(ctx context.Context, secretIDs []uuid.UUID) error {
	if len(secretIDs) == 0 {
		return nil
	}
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE secrets SET last_used_at = now() WHERE id = ANY($1)`, secretIDs)
	return translate(err)
}

// DeleteSecret soft-deletes a secret so its history and audit trail survive.
func (r *SecretRepo) DeleteSecret(ctx context.Context, q Queryer, orgID, secretID uuid.UUID) error {
	if q == nil {
		q = r.db.Pool
	}
	tag, err := q.Exec(ctx, `
		UPDATE secrets s SET deleted_at = now()
		FROM environments e, projects p
		WHERE s.id = $1 AND e.id = s.environment_id AND p.id = e.project_id
		  AND p.organization_id = $2 AND s.deleted_at IS NULL`, secretID, orgID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
