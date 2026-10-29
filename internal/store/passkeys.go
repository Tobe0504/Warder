package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Passkey is one registered WebAuthn credential.
type Passkey struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	Transports      []string
	AAGUID          []byte
	SignCount       int64
	BackupEligible  bool
	BackupState     bool
	Nickname        string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}

// ListPasskeys returns a user's registered credentials, newest last.
func (r *AccountRepo) ListPasskeys(ctx context.Context, userID uuid.UUID) ([]Passkey, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, user_id, credential_id, public_key, attestation_type, transports,
		       aaguid, sign_count, backup_eligible, backup_state, nickname,
		       created_at, last_used_at
		FROM webauthn_credentials
		WHERE user_id = $1
		ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	out := make([]Passkey, 0, 4)
	for rows.Next() {
		var p Passkey
		if err := rows.Scan(&p.ID, &p.UserID, &p.CredentialID, &p.PublicKey,
			&p.AttestationType, &p.Transports, &p.AAGUID, &p.SignCount,
			&p.BackupEligible, &p.BackupState, &p.Nickname,
			&p.CreatedAt, &p.LastUsedAt); err != nil {
			return nil, translate(err)
		}
		out = append(out, p)
	}
	return out, translate(rows.Err())
}

// CountPasskeys reports how many a user has, which is what decides whether a
// password alone is enough to sign in.
func (r *AccountRepo) CountPasskeys(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM webauthn_credentials WHERE user_id = $1`, userID).Scan(&count)
	return count, translate(err)
}

// CreatePasskey stores a newly registered credential.
func (r *AccountRepo) CreatePasskey(ctx context.Context, q Queryer, p *Passkey) error {
	return translate(q.QueryRow(ctx, `
		INSERT INTO webauthn_credentials
			(user_id, credential_id, public_key, attestation_type, transports,
			 aaguid, sign_count, backup_eligible, backup_state, nickname)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		p.UserID, p.CredentialID, p.PublicKey, p.AttestationType, p.Transports,
		p.AAGUID, p.SignCount, p.BackupEligible, p.BackupState, p.Nickname,
	).Scan(&p.ID, &p.CreatedAt))
}

// TouchPasskey advances the stored sign counter after a successful assertion.
//
// The counter only ever moves forward. A WHERE that refuses to go backwards
// means a replayed assertion cannot quietly rewind it and be accepted again.
func (r *AccountRepo) TouchPasskey(ctx context.Context, credentialID []byte, signCount int64) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE webauthn_credentials
		SET sign_count = GREATEST(sign_count, $2), last_used_at = now()
		WHERE credential_id = $1`, credentialID, signCount)
	return translate(err)
}

// DeletePasskey removes one of a user's credentials.
func (r *AccountRepo) DeletePasskey(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx,
		`DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Challenges
// ---------------------------------------------------------------------------

// SaveChallenge stores the server's half of a ceremony in progress, replacing
// any earlier one for the same user and purpose.
//
// One outstanding challenge per purpose: a person who starts registration
// twice should finish the ceremony they are looking at, not an abandoned one.
func (r *AccountRepo) SaveChallenge(
	ctx context.Context, userID uuid.UUID, purpose string, session []byte, expiresAt time.Time,
) error {
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM webauthn_challenges WHERE user_id = $1 AND purpose = $2`, userID, purpose)
	if err != nil {
		return translate(err)
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO webauthn_challenges (user_id, purpose, session, expires_at)
		VALUES ($1, $2, $3, $4)`, userID, purpose, session, expiresAt)
	return translate(err)
}

// ConsumeChallenge returns a challenge and deletes it in the same statement.
//
// Single use by construction: a second attempt finds nothing, so an assertion
// cannot be replayed against the challenge that produced it.
func (r *AccountRepo) ConsumeChallenge(
	ctx context.Context, userID uuid.UUID, purpose string, now time.Time,
) ([]byte, error) {
	var session []byte
	err := r.db.Pool.QueryRow(ctx, `
		DELETE FROM webauthn_challenges
		WHERE user_id = $1 AND purpose = $2 AND expires_at > $3
		RETURNING session`, userID, purpose, now).Scan(&session)
	if err != nil {
		return nil, translate(err)
	}
	return session, nil
}

// ---------------------------------------------------------------------------
// Pending second factor
// ---------------------------------------------------------------------------

// PendingMFA is a password that has been accepted and is waiting on a passkey.
type PendingMFA struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Kind   string
}

// CreatePendingMFA records an accepted password awaiting its second factor.
func (r *AccountRepo) CreatePendingMFA(
	ctx context.Context, userID uuid.UUID, tokenHash []byte, kind string, expiresAt time.Time,
) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO mfa_pending (user_id, token_hash, kind, expires_at)
		VALUES ($1, $2, $3, $4)`, userID, tokenHash, kind, expiresAt)
	return translate(err)
}

// FindPendingMFA resolves a pending token without spending it, so that the
// challenge can be issued and the same token used to answer it.
func (r *AccountRepo) FindPendingMFA(
	ctx context.Context, tokenHash []byte, now time.Time,
) (*PendingMFA, error) {
	p := &PendingMFA{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, user_id, kind FROM mfa_pending
		WHERE token_hash = $1 AND expires_at > $2`, tokenHash, now,
	).Scan(&p.ID, &p.UserID, &p.Kind)
	if err != nil {
		return nil, translate(err)
	}
	return p, nil
}

// ConsumePendingMFA spends the pending token once the factor is satisfied.
func (r *AccountRepo) ConsumePendingMFA(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM mfa_pending WHERE id = $1`, id)
	return translate(err)
}

// ---------------------------------------------------------------------------
// Passwordless sign-in challenges
// ---------------------------------------------------------------------------

// SavePasswordlessChallenge stores a ceremony that names no account yet.
//
// Keyed by an opaque handle rather than a user, because a discoverable
// assertion is how the account becomes known: at this point nobody has said
// who they are, which is exactly the property that stops this endpoint being
// used to ask whether an address exists.
func (r *AccountRepo) SavePasswordlessChallenge(
	ctx context.Context, handle uuid.UUID, session []byte, expiresAt time.Time,
) error {
	// Expired rows are swept opportunistically here rather than by a job: this
	// is the only writer, and the table is small.
	if _, err := r.db.Pool.Exec(ctx,
		`DELETE FROM webauthn_signin_challenges WHERE expires_at < now()`); err != nil {
		return translate(err)
	}

	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO webauthn_signin_challenges (handle, session, expires_at)
		VALUES ($1, $2, $3)`, handle, session, expiresAt)
	return translate(err)
}

// ConsumePasswordlessChallenge returns a challenge and deletes it in the same
// statement, so an assertion cannot be replayed against it.
func (r *AccountRepo) ConsumePasswordlessChallenge(
	ctx context.Context, handle uuid.UUID, now time.Time,
) ([]byte, error) {
	var session []byte
	err := r.db.Pool.QueryRow(ctx, `
		DELETE FROM webauthn_signin_challenges
		WHERE handle = $1 AND expires_at > $2
		RETURNING session`, handle, now).Scan(&session)
	if err != nil {
		return nil, translate(err)
	}
	return session, nil
}

// DeleteAllPasskeys removes every passkey on an account, and any sign-in still
// waiting on one, returning how many keys went.
//
// Only for recovery that an owner vouched for: the person has lost their keys,
// and leaving the keys registered would leave the new password stuck at the
// second-factor step it can no longer pass.
func (r *AccountRepo) DeleteAllPasskeys(ctx context.Context, q Queryer, userID uuid.UUID) (int64, error) {
	tag, err := q.Exec(ctx, `DELETE FROM webauthn_credentials WHERE user_id = $1`, userID)
	if err != nil {
		return 0, translate(err)
	}
	if _, err := q.Exec(ctx, `DELETE FROM mfa_pending WHERE user_id = $1`, userID); err != nil {
		return 0, translate(err)
	}
	return tag.RowsAffected(), nil
}
