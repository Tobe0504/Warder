package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

// handleRecoveryCodeStatus reports how many codes remain.
//
// The codes themselves are never returned here, or anywhere other than the one
// response that creates them. A route that could re-read them would make the
// "written down once" property a lie.
func (s *Server) handleRecoveryCodeStatus(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok || principal.ActorType != domain.ActorHuman {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	remaining, err := s.accounts.CountUnusedRecoveryCodes(r.Context(), principal.ID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"remaining": remaining})
}

// handleIssueRecoveryCodes mints a fresh set and invalidates the old one.
//
// This is the only response in the system that carries recovery codes in
// plaintext. They are not stored anywhere they can be read back, so a person
// who closes the page without saving them has to issue another set, and the
// interface should say so before they close it.
func (s *Server) handleIssueRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok || principal.ActorType != domain.ActorHuman {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	codes, err := credential.NewRecoveryCodes()
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	hashes := make([][]byte, 0, len(codes))
	for _, code := range codes {
		hashes = append(hashes, credential.HashRecoveryCode(code))
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.accounts.ReplaceRecoveryCodes(r.Context(), tx, principal.ID, hashes); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventRecoveryCodesIssued,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      domain.ActorHuman,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata:       map[string]any{"count": len(codes)},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{"codes": codes})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// handleChangePassword replaces the caller's password.
//
// The current password is required even though the caller already holds a
// session, because a session is something a borrowed laptop also holds. This
// is the check that makes the new password mean something.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok || principal.ActorType != domain.ActorHuman {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	var req changePasswordRequest
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

	user, err := s.accounts.GetUser(r.Context(), principal.ID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	valid, err := credential.VerifyPassword(req.CurrentPassword, user.PasswordHash)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}
	if !valid {
		writeError(w, r, s.logger, ErrIncorrectPassword, nil)
		return
	}

	if err := s.setPassword(r, user, req.NewPassword, audit.EventPasswordChanged, nil); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Every other session goes, including this one: the caller signs in again
	// with the password they just chose, which is also how they find out it
	// was stored as typed. A password changed because it may have been seen is
	// not changed until the sessions it protected are gone.
	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}

type resetPasswordRequest struct {
	Email        string `json:"email"`
	RecoveryCode string `json:"recoveryCode"`
	NewPassword  string `json:"newPassword"`
}

// handleResetPassword spends a recovery code to set a new password.
//
// Unauthenticated, because the whole point is that the caller cannot sign in.
// The code is the credential, and it is single use.
//
// Every failure answers identically and takes the same work, so this cannot be
// used to learn whether an address has an account: a wrong address, a wrong
// code and a spent code are one answer.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
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

	email := strings.ToLower(strings.TrimSpace(req.Email))
	clientIP := ClientIP(r, s.cfg.TrustProxyHeaders)

	user, err := s.accounts.GetUserByEmail(r.Context(), email)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrInternal, err)
			return
		}
		// Spend the same work a real attempt would, so that response time does
		// not answer "does this address have an account".
		credential.DummyVerify(req.NewPassword)
		s.logger.Info("recovery rejected", "reason", "unknown account", "client_ip", clientIP)
		writeError(w, r, s.logger, ErrRecoveryFailed, nil)
		return
	}

	hash := credential.HashRecoveryCode(req.RecoveryCode)

	var spent bool
	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		ok, err := s.accounts.ConsumeRecoveryCode(r.Context(), tx, user.ID, hash, clientIP)
		if err != nil {
			return err
		}
		spent = ok
		return nil
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	if !spent {
		credential.DummyVerify(req.NewPassword)
		s.recordRecoveryFailure(r, user.ID, email, clientIP)
		writeError(w, r, s.logger, ErrRecoveryFailed, nil)
		return
	}

	if err := s.setPassword(r, user, req.NewPassword, audit.EventPasswordReset, map[string]any{
		"method": "recovery_code",
	}); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	remaining, err := s.accounts.CountUnusedRecoveryCodes(r.Context(), user.ID)
	if err != nil {
		remaining = 0
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"ok":                true,
		"remainingCodes":    remaining,
		"sessionsSignedOut": true,
	})
}

// setPassword stores a new password and ends every session the old one
// protected.
//
// Shared by changing and resetting, because the part that matters is identical
// in both: a password is not really changed while the sessions it was guarding
// are still usable.
func (s *Server) setPassword(
	r *http.Request,
	user *domain.User,
	password string,
	event audit.EventType,
	metadata map[string]any,
) error {
	hash, err := credential.HashPassword(password)
	if err != nil {
		return err
	}

	// The organization a password event is recorded against is whichever one
	// the person belongs to, or none. A password is not an organization's
	// property, but the trail has to live somewhere a reader will look.
	organizationID := uuid.Nil
	if membership, err := s.accounts.FindPrimaryMembership(r.Context(), user.ID); err == nil {
		organizationID = membership.OrganizationID
	}

	return store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.accounts.UpdatePasswordHash(r.Context(), tx, user.ID, hash); err != nil {
			return err
		}
		if err := s.accounts.RevokeSessionsForUser(r.Context(), tx, user.ID); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: organizationID,
			Type:           event,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      domain.ActorHuman,
			ActorID:        &user.ID,
			ActorLabel:     user.Email,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata:       metadata,
		})
	})
}

// recordRecoveryFailure notes a rejected recovery attempt against a real
// account, which is the one that is worth looking at.
func (s *Server) recordRecoveryFailure(r *http.Request, userID uuid.UUID, email, clientIP string) {
	organizationID := uuid.Nil
	if membership, err := s.accounts.FindPrimaryMembership(r.Context(), userID); err == nil {
		organizationID = membership.OrganizationID
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: organizationID,
		Type:           audit.EventPasswordReset,
		Outcome:        audit.OutcomeDenied,
		ActorType:      domain.ActorHuman,
		ActorID:        &userID,
		ActorLabel:     email,
		IPAddress:      clientIP,
		UserAgent:      r.UserAgent(),
		Metadata:       map[string]any{"reason": "no matching unused code"},
	})
}
