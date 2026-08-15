package store

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tobe0504/Warder/internal/domain"
)

// Invitation is a pending offer of membership.
//
// It never carries the token. The row holds a SHA-256 verifier and a public
// handle, exactly as machine tokens and sessions do, so a database backup does
// not contain anything that can be redeemed.
type Invitation struct {
	ID                  uuid.UUID
	OrganizationID      uuid.UUID
	Email               string
	Name                string
	Role                domain.Role
	MembershipExpiresAt *time.Time
	PublicID            string
	CreatedAt           time.Time
	CreatedBy           *uuid.UUID
	ExpiresAt           time.Time
	AcceptedAt          *time.Time
	RevokedAt           *time.Time
}

// Pending reports whether the invitation can still be redeemed.
func (i *Invitation) Pending(now time.Time) bool {
	return i.AcceptedAt == nil && i.RevokedAt == nil && i.ExpiresAt.After(now)
}

// Status renders the invitation's state for a listing.
func (i *Invitation) Status(now time.Time) string {
	switch {
	case i.AcceptedAt != nil:
		return "ACCEPTED"
	case i.RevokedAt != nil:
		return "REVOKED"
	case !i.ExpiresAt.After(now):
		return "EXPIRED"
	default:
		return "PENDING"
	}
}

// CreateInvitation stores a new invitation and its verifier.
func (r *AccountRepo) CreateInvitation(ctx context.Context, q Queryer, inv *Invitation, tokenHash []byte) (*Invitation, error) {
	if q == nil {
		q = r.db.Pool
	}
	email := strings.ToLower(strings.TrimSpace(inv.Email))

	err := q.QueryRow(ctx, `
		INSERT INTO membership_invitations
			(organization_id, email, name, role, membership_expires_at,
			 public_id, token_hash, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		inv.OrganizationID, email, inv.Name, string(inv.Role), inv.MembershipExpiresAt,
		inv.PublicID, tokenHash, inv.CreatedBy, inv.ExpiresAt,
	).Scan(&inv.ID, &inv.CreatedAt)
	if err != nil {
		return nil, translate(err)
	}
	inv.Email = email
	return inv, nil
}

// ListInvitations returns an organization's invitations, newest first.
func (r *AccountRepo) ListInvitations(ctx context.Context, orgID uuid.UUID) ([]Invitation, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, email, name, role, membership_expires_at, public_id,
		       created_at, created_by, expires_at, accepted_at, revoked_at
		FROM membership_invitations
		WHERE organization_id = $1
		ORDER BY created_at DESC
		LIMIT 200`, orgID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []Invitation
	for rows.Next() {
		inv := Invitation{OrganizationID: orgID}
		var role string
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Name, &role, &inv.MembershipExpiresAt,
			&inv.PublicID, &inv.CreatedAt, &inv.CreatedBy, &inv.ExpiresAt,
			&inv.AcceptedAt, &inv.RevokedAt); err != nil {
			return nil, translate(err)
		}
		inv.Role = domain.Role(role)
		out = append(out, inv)
	}
	return out, translate(rows.Err())
}

// RevokeInvitation withdraws a pending invitation.
func (r *AccountRepo) RevokeInvitation(ctx context.Context, orgID, invitationID uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE membership_invitations SET revoked_at = now()
		WHERE id = $1 AND organization_id = $2
		  AND accepted_at IS NULL AND revoked_at IS NULL`,
		invitationID, orgID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		// Already accepted, already revoked, or another organization's. All
		// three answer the same way: there is nothing here to withdraw.
		return ErrNotFound
	}
	return nil
}

// FindInvitationByPublicID loads the candidate row for a presented token.
//
// It returns the row whatever its state, so the caller can distinguish an
// expired invitation from an unknown one when telling the *invitee* what to do
// — they hold the token, so nothing is disclosed by being specific.
func (r *AccountRepo) FindInvitationByPublicID(ctx context.Context, q Queryer, publicID string) (*Invitation, []byte, error) {
	if q == nil {
		q = r.db.Pool
	}
	inv := &Invitation{PublicID: publicID}
	var role string
	var hash []byte

	err := q.QueryRow(ctx, `
		SELECT id, organization_id, email, name, role, membership_expires_at,
		       token_hash, created_at, created_by, expires_at, accepted_at, revoked_at
		FROM membership_invitations
		WHERE public_id = $1`, publicID,
	).Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Name, &role,
		&inv.MembershipExpiresAt, &hash, &inv.CreatedAt, &inv.CreatedBy,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt)
	if err != nil {
		return nil, nil, translate(err)
	}
	inv.Role = domain.Role(role)
	return inv, hash, nil
}

// MarkInvitationAccepted closes the invitation, conditional on it still being
// open.
//
// This is what makes an invitation single-use. Two requests redeeming the same
// token concurrently both read an open row; only one of these updates matches,
// and the loser is rolled back with its user and membership inserts.
func (r *AccountRepo) MarkInvitationAccepted(ctx context.Context, tx pgx.Tx, invitationID, userID uuid.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE membership_invitations
		SET accepted_at = now(), accepted_user_id = $2
		WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL
		  AND expires_at > now()`,
		invitationID, userID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UserExistsByEmail reports whether an address already has an account.
func (r *AccountRepo) UserExistsByEmail(ctx context.Context, q Queryer, email string) (bool, error) {
	if q == nil {
		q = r.db.Pool
	}
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1))`,
		strings.TrimSpace(email),
	).Scan(&exists)
	return exists, translate(err)
}
