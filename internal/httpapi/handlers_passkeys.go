package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

const (
	purposeRegister = "REGISTER"
	purposeLogin    = "LOGIN"

	// challengeTTL bounds how long a half-finished ceremony stays open. A
	// person is at their keyboard with a key in their hand; minutes is
	// generous.
	challengeTTL = 5 * time.Minute
)

// handleListPasskeys returns the caller's registered keys.
func (s *Server) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.humanPrincipal(w, r)
	if !ok {
		return
	}

	passkeys, err := s.accounts.ListPasskeys(r.Context(), principal.ID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]map[string]any, 0, len(passkeys))
	for _, p := range passkeys {
		out = append(out, map[string]any{
			"id":         p.ID.String(),
			"nickname":   p.Nickname,
			"createdAt":  p.CreatedAt.UTC().Format(time.RFC3339),
			"lastUsedAt": formatTimePtr(p.LastUsedAt),
			"transports": p.Transports,
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"passkeys": out,
		"enabled":  s.webauthn != nil,
	})
}

// handleBeginPasskeyRegistration issues a registration challenge.
func (s *Server) handleBeginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.humanPrincipal(w, r)
	if !ok {
		return
	}
	if s.webauthn == nil {
		writeError(w, r, s.logger, ErrPasskeysUnavailable, nil)
		return
	}

	account, err := s.passkeyAccount(r, principal.ID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Excluding the keys already registered stops an authenticator being
	// enrolled twice, which would read as two factors while being one object.
	exclusions := make([]protocol.CredentialDescriptor, 0, len(account.passkeys))
	for _, c := range account.WebAuthnCredentials() {
		exclusions = append(exclusions, c.Descriptor())
	}

	options, session, err := s.webauthn.BeginRegistration(account,
		webauthn.WithExclusions(exclusions))
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	if err := s.storeChallenge(r, principal.ID, purposeRegister, session); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, options)
}

type finishRegistrationRequest struct {
	Nickname string `json:"nickname"`
	// Credential is the authenticator's response, passed through unchanged.
	Credential json.RawMessage `json:"credential"`
}

// handleFinishPasskeyRegistration verifies the attestation and stores the key.
func (s *Server) handleFinishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.humanPrincipal(w, r)
	if !ok {
		return
	}
	if s.webauthn == nil {
		writeError(w, r, s.logger, ErrPasskeysUnavailable, nil)
		return
	}

	var req finishRegistrationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	nickname := v.requireName("nickname", req.Nickname)
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	session, ok := s.takeChallenge(w, r, principal.ID, purposeRegister)
	if !ok {
		return
	}

	account, err := s.passkeyAccount(r, principal.ID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(req.Credential))
	if err != nil {
		writeError(w, r, s.logger, ErrPasskeyRejected, err)
		return
	}

	created, err := s.webauthn.CreateCredential(account, *session, parsed)
	if err != nil {
		writeError(w, r, s.logger, ErrPasskeyRejected, err)
		return
	}

	record := storedPasskey(principal.ID, nickname, created)
	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.accounts.CreatePasskey(r.Context(), tx, record); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventPasskeyRegistered,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      domain.ActorHuman,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata:       map[string]any{"nickname": nickname},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"id":       record.ID.String(),
		"nickname": record.Nickname,
	})
}

// handleDeletePasskey removes one of the caller's keys.
//
// Removing the last one silently drops the account back to a password alone,
// so the response says how many are left and the interface can say so too.
func (s *Server) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.humanPrincipal(w, r)
	if !ok {
		return
	}

	id, valid := pathUUID(r, "passkeyID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	if err := s.accounts.DeletePasskey(r.Context(), principal.ID, id); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	remaining, err := s.accounts.CountPasskeys(r.Context(), principal.ID)
	if err != nil {
		remaining = 0
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: principal.OrganizationID,
		Type:           audit.EventPasskeyRemoved,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      domain.ActorHuman,
		ActorID:        &principal.ID,
		ActorLabel:     principal.DisplayName,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Metadata:       map[string]any{"remaining": remaining},
	})

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"remaining": remaining})
}

// ---------------------------------------------------------------------------
// The second factor at sign-in
// ---------------------------------------------------------------------------

type pendingRequest struct {
	PendingToken string          `json:"pendingToken"`
	Credential   json.RawMessage `json:"credential"`
}

// handleBeginPasskeyAssertion issues the challenge for a password that has
// already been accepted.
func (s *Server) handleBeginPasskeyAssertion(w http.ResponseWriter, r *http.Request) {
	var req pendingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}
	if s.webauthn == nil {
		writeError(w, r, s.logger, ErrPasskeysUnavailable, nil)
		return
	}

	pending, ok := s.resolvePending(w, r, req.PendingToken)
	if !ok {
		return
	}

	account, err := s.passkeyAccount(r, pending.UserID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	options, session, err := s.webauthn.BeginLogin(account)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	if err := s.storeChallenge(r, pending.UserID, purposeLogin, session); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, options)
}

// handleFinishPasskeyAssertion verifies the assertion and issues the session
// the password alone did not earn.
func (s *Server) handleFinishPasskeyAssertion(w http.ResponseWriter, r *http.Request) {
	var req pendingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}
	if s.webauthn == nil {
		writeError(w, r, s.logger, ErrPasskeysUnavailable, nil)
		return
	}

	pending, ok := s.resolvePending(w, r, req.PendingToken)
	if !ok {
		return
	}

	session, ok := s.takeChallenge(w, r, pending.UserID, purposeLogin)
	if !ok {
		return
	}

	account, err := s.passkeyAccount(r, pending.UserID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(req.Credential))
	if err != nil {
		writeError(w, r, s.logger, ErrPasskeyRejected, err)
		return
	}

	used, err := s.webauthn.ValidateLogin(account, *session, parsed)
	if err != nil {
		s.logger.Info("passkey assertion rejected", "error", err,
			"client_ip", ClientIP(r, s.cfg.TrustProxyHeaders))
		writeError(w, r, s.logger, ErrPasskeyRejected, nil)
		return
	}

	// A counter that failed to advance means the assertion was replayed or the
	// credential was cloned. The library reports it; refusing is the only safe
	// reading, and it is loud in the trail because a real clone is an incident.
	if used.Authenticator.CloneWarning {
		s.recordPasskeyFailure(r, pending.UserID, "sign counter did not advance")
		writeError(w, r, s.logger, ErrPasskeyRejected, nil)
		return
	}

	if err := s.accounts.TouchPasskey(r.Context(), used.ID, int64(used.Authenticator.SignCount)); err != nil {
		s.logger.Warn("could not advance the passkey sign counter", "error", err)
	}
	if err := s.accounts.ConsumePendingMFA(r.Context(), pending.ID); err != nil {
		s.logger.Warn("could not clear the pending second factor", "error", err)
	}

	s.completeSignIn(w, r, account.user, pending.Kind, map[string]any{"second_factor": "passkey"})
}

type pendingRecoveryRequest struct {
	PendingToken string `json:"pendingToken"`
	RecoveryCode string `json:"recoveryCode"`
}

// handleFinishWithRecoveryCode satisfies the second factor with a recovery code
// in place of the passkey.
//
// Without this, a lost passkey is a lost account: resetting the password does
// not remove the passkey, so the new password would stop at the same step. The
// password has already been checked to get the pending token, so the code here
// is the second factor, exactly as the passkey would have been, and it is spent.
func (s *Server) handleFinishWithRecoveryCode(w http.ResponseWriter, r *http.Request) {
	var req pendingRecoveryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	pending, ok := s.resolvePending(w, r, req.PendingToken)
	if !ok {
		return
	}

	user, err := s.accounts.GetUser(r.Context(), pending.UserID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	clientIP := ClientIP(r, s.cfg.TrustProxyHeaders)
	hash := credential.HashRecoveryCode(req.RecoveryCode)

	var spent bool
	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		ok, err := s.accounts.ConsumeRecoveryCode(r.Context(), tx, user.ID, hash, clientIP)
		spent = ok
		return err
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if !spent {
		s.recordRecoveryFailure(r, user.ID, user.Email, clientIP)
		writeError(w, r, s.logger, ErrRecoveryCodeRejected, nil)
		return
	}

	if err := s.accounts.ConsumePendingMFA(r.Context(), pending.ID); err != nil {
		s.logger.Warn("could not clear the pending second factor", "error", err)
	}

	s.completeSignIn(w, r, user, pending.Kind, map[string]any{"second_factor": "recovery_code"})
}

// ---------------------------------------------------------------------------
// Shared pieces
// ---------------------------------------------------------------------------

func (s *Server) humanPrincipal(w http.ResponseWriter, r *http.Request) (*domain.Principal, bool) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok || principal.ActorType != domain.ActorHuman {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return nil, false
	}
	return principal, true
}

func (s *Server) passkeyAccount(r *http.Request, userID uuid.UUID) (passkeyUser, error) {
	user, err := s.accounts.GetUser(r.Context(), userID)
	if err != nil {
		return passkeyUser{}, err
	}
	passkeys, err := s.accounts.ListPasskeys(r.Context(), userID)
	if err != nil {
		return passkeyUser{}, err
	}
	return passkeyUser{user: user, passkeys: passkeys}, nil
}

func (s *Server) storeChallenge(
	r *http.Request, userID uuid.UUID, purpose string, session *webauthn.SessionData,
) error {
	encoded, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.accounts.SaveChallenge(r.Context(), userID, purpose, encoded,
		s.now().Add(challengeTTL))
}

func (s *Server) takeChallenge(
	w http.ResponseWriter, r *http.Request, userID uuid.UUID, purpose string,
) (*webauthn.SessionData, bool) {
	raw, err := s.accounts.ConsumeChallenge(r.Context(), userID, purpose, s.now())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrChallengeExpired, nil)
			return nil, false
		}
		writeError(w, r, s.logger, translateError(err), err)
		return nil, false
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(raw, &session); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return nil, false
	}
	return &session, true
}

func (s *Server) resolvePending(
	w http.ResponseWriter, r *http.Request, presented string,
) (*store.PendingMFA, bool) {
	token := strings.TrimSpace(presented)
	if token == "" {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return nil, false
	}

	pending, err := s.accounts.FindPendingMFA(r.Context(), credential.Hash(token), s.now())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrUnauthorized, nil)
			return nil, false
		}
		writeError(w, r, s.logger, translateError(err), err)
		return nil, false
	}
	return pending, true
}

func (s *Server) recordPasskeyFailure(r *http.Request, userID uuid.UUID, reason string) {
	organizationID := uuid.Nil
	if membership, err := s.accounts.FindPrimaryMembership(r.Context(), userID); err == nil {
		organizationID = membership.OrganizationID
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: organizationID,
		Type:           audit.EventPasskeyRejected,
		Outcome:        audit.OutcomeDenied,
		ActorType:      domain.ActorHuman,
		ActorID:        &userID,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Reason:         reason,
	})
}
