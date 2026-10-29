package store

import (
	"context"

	"github.com/google/uuid"
)

// ReplaceRecoveryCodes swaps a user's entire set for a new one.
//
// Issuing a set invalidates every code from the previous set, used or not.
// Two live sets would mean a code the person believes they destroyed still
// opening the account, which is the opposite of what regenerating them is for.
func (r *AccountRepo) ReplaceRecoveryCodes(
	ctx context.Context,
	q Queryer,
	userID uuid.UUID,
	hashes [][]byte,
) error {
	if _, err := q.Exec(ctx, `DELETE FROM recovery_codes WHERE user_id = $1`, userID); err != nil {
		return translate(err)
	}

	// One statement rather than ten round trips: unnest expands the array of
	// hashes into rows.
	_, err := q.Exec(ctx, `
		INSERT INTO recovery_codes (user_id, code_hash)
		SELECT $1, hash FROM unnest($2::bytea[]) AS hash`,
		userID, hashes)
	return translate(err)
}

// ConsumeRecoveryCode spends a code, and reports whether there was one to
// spend.
//
// The update is the check: a single statement marks the row used only if it is
// both this user's and not already spent. Two requests racing with the same
// code cannot both succeed, because the second finds nothing left to update.
func (r *AccountRepo) ConsumeRecoveryCode(
	ctx context.Context,
	q Queryer,
	userID uuid.UUID,
	hash []byte,
	ip string,
) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE recovery_codes
		SET used_at = now(), used_ip = $3
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`,
		userID, hash, nullInet(ip))
	if err != nil {
		return false, translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

// CountUnusedRecoveryCodes reports how many a user has left.
func (r *AccountRepo) CountUnusedRecoveryCodes(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM recovery_codes
		WHERE user_id = $1 AND used_at IS NULL`, userID).Scan(&count)
	if err != nil {
		return 0, translate(err)
	}
	return count, nil
}
