package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

type signupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleSignup creates an account that belongs to no organization yet.
//
// Signing up and founding an organization used to be the same request, which
// made an account something only a first owner could have. That made two
// ordinary situations inexpressible: somebody invited to an organization they
// did not create, and somebody between organizations. A person exists first
// and belongs afterwards.
//
// The response carries a session, because the next thing this person does is
// either create an organization or open an invitation, and both need one.
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	name := v.requireName("name", req.Name)

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") || len(email) > 320 {
		v.add("email", "Enter a valid email address.")
	}
	if len(req.Password) < minPasswordLength {
		v.add("password", "Use at least 12 characters.")
	}
	if len(req.Password) > maxPasswordLength {
		v.add("password", "That password is too long.")
	}
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	passwordHash, err := credential.HashPassword(req.Password)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	clientIP := ClientIP(r, s.cfg.TrustProxyHeaders)

	// Issued here rather than offered later, because "later" is a screen
	// people close. Warder sends no email, so these codes are the only
	// self-service way back into an account whose password is forgotten, and
	// an account created without them has no route back at all.
	codes, err := credential.NewRecoveryCodes()
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}
	hashes := make([][]byte, 0, len(codes))
	for _, code := range codes {
		hashes = append(hashes, credential.HashRecoveryCode(code))
	}

	var user *domain.User
	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var err error
		if user, err = s.accounts.CreateUser(r.Context(), tx, email, name, passwordHash); err != nil {
			return err
		}
		return s.accounts.ReplaceRecoveryCodes(r.Context(), tx, user.ID, hashes)
	})
	if err != nil {
		// A taken address answers as itself rather than as a generic failure:
		// the person is standing at a form they can correct, and the address
		// is one they just typed.
		if errors.Is(err, store.ErrConflict) {
			writeError(w, r, s.logger, ErrAccountExists, nil)
			return
		}
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	now := s.now()
	// Browser only: the CLI signs in, it does not sign people up.
	kind := "BROWSER"
	ttl := s.cfg.SessionTTL
	if kind == "CLI" {
		ttl = s.cfg.CLISessionTTL
	}

	token, err := credential.Mint(credential.KindSession)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	// uuid.Nil: this session belongs to no organization, and the route guard
	// is what keeps it away from everything that needs one.
	session := &store.Session{
		UserID:         user.ID,
		OrganizationID: uuid.Nil,
		Kind:           kind,
		ExpiresAt:      now.Add(ttl),
	}
	if err := s.accounts.CreateSession(r.Context(), session, token.PublicID, token.Hash, clientIP, r.UserAgent()); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	// Recorded against no organization, because there is not one yet. The
	// trail for this account begins when it joins something.
	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: uuid.Nil,
		Type:           audit.EventLogin,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      domain.ActorHuman,
		ActorID:        &user.ID,
		ActorLabel:     user.Email,
		CredentialID:   &session.ID,
		IPAddress:      clientIP,
		UserAgent:      r.UserAgent(),
		Metadata:       map[string]any{"session_kind": kind, "signup": true},
	})

	// The codes travel in this one response and are never readable again.
	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"sessionToken": token.Secret,
		"expiresAt":    session.ExpiresAt.UTC().Format(time.RFC3339),
		"user": userResponse{
			ID:    user.ID.String(),
			Email: user.Email,
			Name:  user.Name,
		},
		"recoveryCodes": codes,
	})
}
