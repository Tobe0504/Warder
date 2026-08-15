package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// minPasswordLength follows current NIST guidance: length is what matters, and
// composition rules mostly produce predictable substitutions. Long passphrases
// are accepted up to a bound that keeps Argon2 from becoming a denial-of-
// service vector.
const (
	minPasswordLength = 12
	maxPasswordLength = 256
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Kind selects a browser session or a longer-lived CLI login.
	Kind string `json:"kind"`
}

type loginResponse struct {
	// SessionToken is returned to the BFF, which places it in an HttpOnly
	// cookie. It is never rendered into a page or handed to client JavaScript.
	SessionToken string       `json:"sessionToken"`
	ExpiresAt    string       `json:"expiresAt"`
	User         userResponse `json:"user"`
}

type userResponse struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	OrganizationID string `json:"organizationId"`
	Organization   string `json:"organization"`
	Role           string `json:"role"`
}

// handleLogin authenticates a human.
//
// Every failure below returns the same response and takes approximately the
// same time. An unknown address, a wrong password, a disabled account, and an
// expired membership are indistinguishable to the caller, so this endpoint
// cannot be used to enumerate who has an account here.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	kind := "BROWSER"
	if strings.EqualFold(req.Kind, "cli") {
		kind = "CLI"
	}

	clientIP := ClientIP(r, s.cfg.TrustProxyHeaders)
	email := strings.ToLower(strings.TrimSpace(req.Email))

	if email == "" || req.Password == "" || len(req.Password) > maxPasswordLength {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	user, err := s.accounts.GetUserByEmail(r.Context(), email)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrInternal, err)
			return
		}
		// Spend the same work as a real verification so that response time does
		// not answer "does this address have an account".
		credential.DummyVerify(req.Password)
		s.recordLoginFailure(r, uuid.Nil, email, clientIP, "unknown account")
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	valid, err := credential.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}
	if !valid {
		s.recordLoginFailure(r, user.ID, email, clientIP, "incorrect password")
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	now := s.now()
	if user.DisabledAt != nil && !user.DisabledAt.After(now) {
		s.recordLoginFailure(r, user.ID, email, clientIP, "account disabled")
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	membership, err := s.accounts.FindPrimaryMembership(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// The membership has expired or been revoked — the contractor whose
			// access ended. The password is still correct; it now confers
			// nothing, and no credential in the organization had to change.
			s.recordLoginFailure(r, user.ID, email, clientIP, "no active membership")
			writeError(w, r, s.logger, ErrUnauthorized, nil)
			return
		}
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	// A successful login is the natural moment to upgrade a hash written under
	// older Argon2 parameters, since it is the only time the password is known.
	if credential.NeedsRehash(user.PasswordHash) {
		if upgraded, err := credential.HashPassword(req.Password); err == nil {
			if err := s.accounts.UpdatePasswordHash(r.Context(), user.ID, upgraded); err != nil {
				s.logger.Warn("could not upgrade password hash", "error", err)
			}
		}
	}

	ttl := s.cfg.SessionTTL
	if kind == "CLI" {
		ttl = s.cfg.CLISessionTTL
	}

	token, err := credential.Mint(credential.KindSession)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	session := &store.Session{
		UserID:         user.ID,
		OrganizationID: membership.OrganizationID,
		Kind:           kind,
		ExpiresAt:      now.Add(ttl),
	}
	if err := s.accounts.CreateSession(r.Context(), session, token.PublicID, token.Hash, clientIP, r.UserAgent()); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	org, err := s.accounts.GetOrganization(r.Context(), membership.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: membership.OrganizationID,
		Type:           audit.EventLogin,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      domain.ActorHuman,
		ActorID:        &user.ID,
		ActorLabel:     user.Email,
		CredentialID:   &session.ID,
		IPAddress:      clientIP,
		UserAgent:      r.UserAgent(),
		Metadata:       map[string]any{"session_kind": kind},
	})

	writeJSON(w, r, s.logger, http.StatusOK, loginResponse{
		SessionToken: token.Secret,
		ExpiresAt:    session.ExpiresAt.UTC().Format(time.RFC3339),
		User: userResponse{
			ID:             user.ID.String(),
			Email:          user.Email,
			Name:           user.Name,
			OrganizationID: org.ID.String(),
			Organization:   org.Name,
			Role:           string(membership.Role),
		},
	})
}

func (s *Server) recordLoginFailure(r *http.Request, userID uuid.UUID, email, clientIP, reason string) {
	ev := audit.Event{
		Type:       audit.EventLoginFailed,
		Outcome:    audit.OutcomeDenied,
		ActorType:  domain.ActorHuman,
		ActorLabel: email,
		IPAddress:  clientIP,
		UserAgent:  r.UserAgent(),
		Reason:     reason,
	}
	if userID != uuid.Nil {
		ev.ActorID = &userID

		// The event needs an organization to be stored against. A failure for
		// an address with no resolvable organization is logged rather than
		// stored, since there is no tenant it belongs to.
		if membership, err := s.accounts.FindPrimaryMembership(r.Context(), userID); err == nil {
			ev.OrganizationID = membership.OrganizationID
			s.audit.Record(r.Context(), ev)
			return
		}
	}

	s.logger.Warn("login failed",
		"reason", reason, "client_ip", clientIP, "user_agent", r.UserAgent())
}

// handleLogout revokes the current session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	if err := s.accounts.RevokeSession(r.Context(), principal.CredentialID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: principal.OrganizationID,
		Type:           audit.EventLogout,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      domain.ActorHuman,
		ActorID:        &principal.ID,
		ActorLabel:     principal.DisplayName,
		CredentialID:   &principal.CredentialID,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
	})

	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}

// handleSession returns the current user, which the BFF uses to render the
// shell without holding user state of its own.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	user, err := s.accounts.GetUser(r.Context(), principal.ID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	org, err := s.accounts.GetOrganization(r.Context(), principal.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, userResponse{
		ID:             user.ID.String(),
		Email:          user.Email,
		Name:           user.Name,
		OrganizationID: org.ID.String(),
		Organization:   org.Name,
		Role:           string(principal.Role),
	})
}

type createOrganizationRequest struct {
	OrganizationName string `json:"organizationName"`
	Slug             string `json:"slug"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	Password         string `json:"password"`
}

// handleCreateOrganization bootstraps an organization with its first owner.
//
// This is open in the MVP so a deployment can be brought up. A real deployment
// should gate it — an invitation, an allowlisted domain, or an operator-only
// path — because as written, anyone who can reach the API can create a tenant.
// It is called out in docs/security/limitations.md rather than left implicit.
func (s *Server) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req createOrganizationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	orgName := v.requireName("organizationName", req.OrganizationName)
	slug := v.requireSlug("slug", req.Slug)
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

	var org *domain.Organization
	var user *domain.User

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var err error
		if org, err = s.accounts.CreateOrganization(r.Context(), tx, orgName, slug); err != nil {
			return err
		}
		if user, err = s.accounts.CreateUser(r.Context(), tx, email, name, passwordHash); err != nil {
			return err
		}
		if _, err = s.accounts.CreateMembership(r.Context(), tx, &domain.Membership{
			OrganizationID: org.ID,
			UserID:         user.ID,
			Role:           domain.RoleOwner,
		}); err != nil {
			return err
		}

		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: org.ID,
			Type:           audit.EventUserInvited,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      domain.ActorHuman,
			ActorID:        &user.ID,
			ActorLabel:     email,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata:       map[string]any{"role": string(domain.RoleOwner), "bootstrap": true},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"organizationId": org.ID.String(),
		"slug":           org.Slug,
		"userId":         user.ID.String(),
	})
}

// handleListMembers returns the organization's members.
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{}) {
		return
	}

	members, err := s.accounts.ListMembers(r.Context(), principal.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{
			"membershipId": m.MembershipID.String(),
			"userId":       m.UserID.String(),
			"email":        m.Email,
			"name":         m.Name,
			"role":         string(m.Role),
			"createdAt":    m.CreatedAt.UTC().Format(time.RFC3339),
			"expiresAt":    formatTimePtr(m.ExpiresAt),
			"revokedAt":    formatTimePtr(m.RevokedAt),
			"active":       (&domain.Membership{ExpiresAt: m.ExpiresAt, RevokedAt: m.RevokedAt}).Active(s.now()),
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"members": out})
}

// handleRemoveMember ends a membership and every session the person holds.
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageOrganization, authzTarget{}) {
		return
	}

	membershipID, valid := pathUUID(r, "membershipID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	members, err := s.accounts.ListMembers(r.Context(), principal.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	var target *store.Member
	for i := range members {
		if members[i].MembershipID == membershipID {
			target = &members[i]
			break
		}
	}
	if target == nil {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	if err := s.accounts.RevokeMembership(r.Context(), principal.OrganizationID, membershipID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Revoking the membership alone would leave existing sessions working until
	// they expired. Removing someone has to take effect on their next request.
	if err := s.accounts.RevokeSessionsForUser(r.Context(), target.UserID); err != nil {
		s.logger.Error("membership revoked but sessions could not be ended",
			"user_id", target.UserID.String(), "error", err)
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: principal.OrganizationID,
		Type:           audit.EventUserRemoved,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      principal.ActorType,
		ActorID:        &principal.ID,
		ActorLabel:     principal.DisplayName,
		CredentialID:   &principal.CredentialID,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Metadata:       map[string]any{"removed_user_id": target.UserID.String()},
	})

	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
