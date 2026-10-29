package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

// adminRecoveryTTL is how long an issued link stays usable. Short, because it
// travels over whatever channel the two people already use, and a link sitting
// unused in a chat history is a standing way into somebody's account.
const adminRecoveryTTL = 2 * time.Hour

// handleIssueAccountRecovery mints a one-time link an owner hands to a member.
//
// This is the only path in Warder by which one person restores another's
// access, and it is a genuine privilege: whoever uses the link sets the
// password, so an owner who issues one for somebody else can sign in as them.
// Guarded by MANAGE_ORGANIZATION, which owners hold and admins do not, and
// recorded loudly on both sides.
//
// The alternative was to let people be permanently locked out, which with no
// email and no passkey is where the ladder otherwise ends.
func (s *Server) handleIssueAccountRecovery(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.humanPrincipal(w, r)
	if !ok {
		return
	}

	membershipID, valid := pathUUID(r, "membershipID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	if !s.allow(w, r, principal, domain.CapManageOrganization, authzTarget{}) {
		return
	}

	member, err := s.accounts.GetMembershipByID(r.Context(), principal.OrganizationID, membershipID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Issuing one for yourself is not recovery, it is a password change you
	// can already make, and allowing it would turn a borrowed session into a
	// way to lock the real owner out.
	if member.UserID == principal.ID {
		writeError(w, r, s.logger, ErrSelfRecovery, nil)
		return
	}

	token, err := credential.Mint(credential.KindRecovery)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	rec := &store.AccountRecovery{
		UserID:         member.UserID,
		OrganizationID: principal.OrganizationID,
		IssuedBy:       principal.ID,
		TokenPrefix:    token.PublicID,
		ExpiresAt:      s.now().Add(adminRecoveryTTL),
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.accounts.CreateAccountRecovery(r.Context(), tx, rec, token.Hash); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventAccountRecoveryIssued,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      domain.ActorHuman,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata: map[string]any{
				"for_user_id": member.UserID.String(),
				"expires_at":  rec.ExpiresAt.UTC().Format(time.RFC3339),
			},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// The token travels in this one response and is not recoverable after it.
	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"token":     token.Secret,
		"expiresAt": rec.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

type redeemRecoveryRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// handleRedeemAccountRecovery spends an owner-issued link to set a password.
//
// Unauthenticated by necessity: the person holding it cannot sign in. The
// token is the whole authority and it is single use.
func (s *Server) handleRedeemAccountRecovery(w http.ResponseWriter, r *http.Request) {
	var req redeemRecoveryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	if len(req.NewPassword) < minPasswordLength {
		v.add("newPassword", "Use at least 12 characters.")
	}
	if len(req.NewPassword) > maxPasswordLength {
		v.add("newPassword", "That password is too long.")
	}
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	presented := strings.TrimSpace(req.Token)
	kind, prefix, err := credential.Parse(presented)
	if err != nil || kind != credential.KindRecovery {
		writeError(w, r, s.logger, ErrRecoveryFailed, nil)
		return
	}

	rec, hash, err := s.accounts.FindAccountRecoveryByPrefix(r.Context(), prefix)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrRecoveryFailed, nil)
			return
		}
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Constant time, and before any state is inspected, so a wrong token and a
	// spent one take the same path.
	if !credential.Verify(presented, hash) {
		writeError(w, r, s.logger, ErrRecoveryFailed, nil)
		return
	}
	if !rec.Pending(s.now()) {
		writeError(w, r, s.logger, ErrRecoveryFailed, nil)
		return
	}

	// The owner's standing to vouch has to still exist. A link issued by
	// somebody who has since left the organization is refused, which stops a
	// departing owner leaving themselves a way back into a colleague's
	// account.
	if _, err := s.accounts.GetMembership(r.Context(), rec.OrganizationID, rec.IssuedBy); err != nil {
		writeError(w, r, s.logger, ErrRecoveryFailed, nil)
		return
	}

	user, err := s.accounts.GetUser(r.Context(), rec.UserID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	clientIP := ClientIP(r, s.cfg.TrustProxyHeaders)

	// The link is the last rung, for somebody who has lost their passkeys too.
	// A new password alone would stop at the passkey step it cannot pass, so
	// the keys go in the same transaction that spends the link, and the person
	// registers fresh ones once they are back in.
	var spent bool
	var passkeysRemoved int64
	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		ok, err := s.accounts.MarkAccountRecoveryUsed(r.Context(), tx, rec.ID, clientIP)
		if err != nil || !ok {
			return err
		}
		spent = true
		passkeysRemoved, err = s.accounts.DeleteAllPasskeys(r.Context(), tx, user.ID)
		return err
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if !spent {
		writeError(w, r, s.logger, ErrRecoveryFailed, nil)
		return
	}

	if err := s.setPassword(r, user, req.NewPassword, audit.EventPasswordReset, map[string]any{
		"method":           "admin_recovery",
		"issued_by":        rec.IssuedBy.String(),
		"passkeys_removed": passkeysRemoved,
	}); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"ok":                true,
		"email":             user.Email,
		"sessionsSignedOut": true,
		"passkeysRemoved":   passkeysRemoved,
	})
}
