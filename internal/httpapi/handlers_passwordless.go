package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/Tobe0504/Warder/internal/store"
)

// passwordlessChallengeTTL bounds a sign-in ceremony that has no account
// attached to it yet.
const passwordlessChallengeTTL = 5 * time.Minute

type passwordlessRequest struct {
	// Handle identifies the challenge issued by the begin step. It is not a
	// credential: it names a challenge and nothing else.
	Handle     string          `json:"handle"`
	Kind       string          `json:"kind"`
	Credential json.RawMessage `json:"credential"`
}

// handleBeginPasswordlessSignIn issues a challenge with no account named.
//
// A discoverable passkey carries the account handle inside it, so the browser
// can answer without anybody typing an address. That is the point: there is no
// password step, and no address to enumerate.
func (s *Server) handleBeginPasswordlessSignIn(w http.ResponseWriter, r *http.Request) {
	if s.webauthn == nil {
		writeError(w, r, s.logger, ErrPasskeysUnavailable, nil)
		return
	}

	options, session, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	encoded, err := json.Marshal(session)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	// Stored against the nil user, because no account is known yet. The handle
	// is what ties the finish step back to this challenge.
	handle := uuid.New()
	if err := s.accounts.SavePasswordlessChallenge(r.Context(), handle, encoded,
		s.now().Add(passwordlessChallengeTTL)); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"handle":  handle.String(),
		"options": options,
	})
}

// handleFinishPasswordlessSignIn verifies a discoverable assertion and signs
// the account in.
//
// Refused unless the account could still get back in without this passkey: a
// second passkey, or recovery codes. Passwordless with one key and no codes is
// one lost phone away from an account nobody can reach, and Warder has no
// email to rescue it with.
func (s *Server) handleFinishPasswordlessSignIn(w http.ResponseWriter, r *http.Request) {
	var req passwordlessRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}
	if s.webauthn == nil {
		writeError(w, r, s.logger, ErrPasskeysUnavailable, nil)
		return
	}

	handle, err := uuid.Parse(req.Handle)
	if err != nil {
		writeError(w, r, s.logger, ErrChallengeExpired, nil)
		return
	}

	raw, err := s.accounts.ConsumePasswordlessChallenge(r.Context(), handle, s.now())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrChallengeExpired, nil)
			return
		}
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(raw, &session); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(req.Credential))
	if err != nil {
		writeError(w, r, s.logger, ErrPasskeyRejected, err)
		return
	}

	// The library hands back the account handle the authenticator asserted,
	// and this is where it is turned into a Warder account. Nothing the client
	// sent names the account.
	var account passkeyUser
	used, err := s.webauthn.ValidateDiscoverableLogin(
		func(rawID, userHandle []byte) (webauthn.User, error) {
			userID, err := uuid.FromBytes(userHandle)
			if err != nil {
				return nil, err
			}
			account, err = s.passkeyAccount(r, userID)
			if err != nil {
				return nil, err
			}
			return account, nil
		}, session, parsed)
	if err != nil {
		s.logger.Info("passwordless assertion rejected", "error", err,
			"client_ip", ClientIP(r, s.cfg.TrustProxyHeaders))
		writeError(w, r, s.logger, ErrPasskeyRejected, nil)
		return
	}

	if used.Authenticator.CloneWarning {
		s.recordPasskeyFailure(r, account.user.ID, "sign counter did not advance")
		writeError(w, r, s.logger, ErrPasskeyRejected, nil)
		return
	}

	// The lockout guard. One passkey and no codes means this account has
	// exactly one way in, and losing the device ends it permanently.
	if len(account.passkeys) < 2 {
		codes, err := s.accounts.CountUnusedRecoveryCodes(r.Context(), account.user.ID)
		if err != nil {
			writeError(w, r, s.logger, ErrInternal, err)
			return
		}
		if codes == 0 {
			writeError(w, r, s.logger, ErrPasswordlessUnsafe, nil)
			return
		}
	}

	if err := s.accounts.TouchPasskey(r.Context(), used.ID, int64(used.Authenticator.SignCount)); err != nil {
		s.logger.Warn("could not advance the passkey sign counter", "error", err)
	}

	kind := "BROWSER"
	if req.Kind == "cli" {
		kind = "CLI"
	}

	s.completeSignIn(w, r, account.user, kind, map[string]any{
		"session_kind": kind,
		"factor":       "passkey_only",
	})
}
