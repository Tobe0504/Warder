package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AccountRecovery is a link an owner issued so a member can set a new
// password.
type AccountRecovery struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	IssuedBy       uuid.UUID
	TokenPrefix    string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	UsedAt         *time.Time
	RevokedAt      *time.Time
}

// Pending reports whether this link can still be used.
func (a *AccountRecovery) Pending(now time.Time) bool {
	return a.UsedAt == nil && a.RevokedAt == nil && a.ExpiresAt.After(now)
}

// CreateAccountRecovery issues a link, superseding any that is still open.
//
// Superseding rather than refusing: an owner reissuing has decided the first
// link is lost, and leaving it live would mean two ways into one account.
func (r *AccountRepo) CreateAccountRecovery(
	ctx context.Context, q Queryer, rec *AccountRecovery, tokenHash []byte,
) error {
	_, err := q.Exec(ctx, `
		UPDATE account_recoveries SET revoked_at = now()
		WHERE user_id = $1 AND used_at IS NULL AND revoked_at IS NULL`, rec.UserID)
	if err != nil {
		return translate(err)
	}

	return translate(q.QueryRow(ctx, `
		INSERT INTO account_recoveries
			(user_id, organization_id, issued_by, token_hash, token_prefix, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		rec.UserID, rec.OrganizationID, rec.IssuedBy, tokenHash, rec.TokenPrefix, rec.ExpiresAt,
	).Scan(&rec.ID, &rec.CreatedAt))
}

// FindAccountRecoveryByPrefix loads the candidate row for a presented token.
// Verification of the secret half happens in the caller, in constant time.
func (r *AccountRepo) FindAccountRecoveryByPrefix(
	ctx context.Context, prefix string,
) (*AccountRecovery, []byte, error) {
	rec := &AccountRecovery{}
	var hash []byte
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, user_id, organization_id, issued_by, token_hash, token_prefix,
		       created_at, expires_at, used_at, revoked_at
		FROM account_recoveries WHERE token_prefix = $1`, prefix,
	).Scan(&rec.ID, &rec.UserID, &rec.OrganizationID, &rec.IssuedBy, &hash,
		&rec.TokenPrefix, &rec.CreatedAt, &rec.ExpiresAt, &rec.UsedAt, &rec.RevokedAt)
	if err != nil {
		return nil, nil, translate(err)
	}
	return rec, hash, nil
}

// MarkAccountRecoveryUsed spends a link, and reports whether there was one to
// spend. The update is the check, so two requests racing cannot both succeed.
func (r *AccountRepo) MarkAccountRecoveryUsed(
	ctx context.Context, q Queryer, id uuid.UUID, ip string,
) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE account_recoveries
		SET used_at = now(), used_ip = $2
		WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()`,
		id, nullInet(ip))
	if err != nil {
		return false, translate(err)
	}
	return tag.RowsAffected() == 1, nil
}
