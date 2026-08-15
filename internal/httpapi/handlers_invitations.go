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

// invitationTTL bounds how long an invite link works.
//
// Short on purpose. An invitation is a way into the organization, and the most
// likely place for one to end up is a chat thread nobody prunes. A week is long
// enough to survive a holiday and short enough that a forgotten link closes on
// its own.
const invitationTTL = 7 * 24 * time.Hour

type createInvitationRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	// ExpiresAt bounds the membership that acceptance creates, not the
	// invitation. This is the contractor workflow: set a date now, and access
	// ends on its own with no credential rotation.
	ExpiresAt string `json:"expiresAt"`
}

// handleCreateInvitation issues a single-use invitation to join.
//
// The response contains the token exactly once. Nothing is emailed from here:
// this service has no mail transport, and adding one would put a credential
// into an outbound queue and a third party's logs. The caller receives the
// token and is responsible for delivering it.
func (s *Server) handleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageOrganization, authzTarget{}) {
		return
	}

	var req createInvitationRequest
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

	role := domain.Role(strings.ToUpper(strings.TrimSpace(req.Role)))
	if !domain.ValidRole(role) {
		v.add("role", "Choose OWNER, ADMIN, DEVELOPER, or VIEWER.")
	}
	membershipExpiry := v.futureTime("expiresAt", req.ExpiresAt, s.now())

	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	// One account per address in this build, so an invitation to an address
	// that already has one could never be redeemed. Refusing here gives the
	// owner a comprehensible error instead of a failure the invitee discovers.
	exists, err := s.accounts.UserExistsByEmail(r.Context(), nil, email)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if exists {
		writeError(w, r, s.logger, ErrAccountExists, nil)
		return
	}

	token, err := credential.Mint(credential.KindInvite)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	expiresAt := s.now().Add(invitationTTL)
	invitation := &store.Invitation{
		OrganizationID:      principal.OrganizationID,
		Email:               email,
		Name:                name,
		Role:                role,
		MembershipExpiresAt: membershipExpiry,
		PublicID:            token.PublicID,
		CreatedBy:           &principal.ID,
		ExpiresAt:           expiresAt,
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if _, err := s.accounts.CreateInvitation(r.Context(), tx, invitation, token.Hash); err != nil {
			return err
		}

		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventUserInvited,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			// The invited address and role are recorded. The token is not, here
			// or anywhere else.
			Metadata: map[string]any{
				"invitation_id":      invitation.ID.String(),
				"invited_email":      email,
				"role":               string(role),
				"membership_expires": membershipExpiry != nil,
			},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"invitationId": invitation.ID.String(),
		"email":        email,
		"role":         string(role),
		"expiresAt":    expiresAt.UTC().Format(time.RFC3339),
		// Shown once, never retrievable again.
		"token": token.Secret,
	})
}

// handleListInvitations returns the organization's invitations.
func (s *Server) handleListInvitations(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{}) {
		return
	}

	invitations, err := s.accounts.ListInvitations(r.Context(), principal.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	now := s.now()
	out := make([]map[string]any, 0, len(invitations))
	for _, inv := range invitations {
		out = append(out, map[string]any{
			"id":     inv.ID.String(),
			"email":  inv.Email,
			"name":   inv.Name,
			"role":   string(inv.Role),
			"status": inv.Status(now),
			// The public handle, so a listing can identify an invitation
			// without anyone holding the token that redeems it.
			"display":   credential.Display(credential.KindInvite, inv.PublicID),
			"createdAt": inv.CreatedAt.UTC().Format(time.RFC3339),
			"expiresAt": inv.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"invitations": out})
}

// handleRevokeInvitation withdraws a pending invitation.
func (s *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageOrganization, authzTarget{}) {
		return
	}

	invitationID, err := uuid.Parse(r.PathValue("invitationID"))
	if err != nil {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	if err := s.accounts.RevokeInvitation(r.Context(), principal.OrganizationID, invitationID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: principal.OrganizationID,
		Type:           audit.EventInvitationRevoked,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      principal.ActorType,
		ActorID:        &principal.ID,
		ActorLabel:     principal.DisplayName,
		CredentialID:   &principal.CredentialID,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Metadata:       map[string]any{"invitation_id": invitationID.String()},
	})

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Acceptance
// ---------------------------------------------------------------------------

type acceptInvitationRequest struct {
	Token string `json:"token"`
	// Name is the invitee's own. The inviter's suggestion is only a default —
	// people should control how their name is written.
	Name     string `json:"name"`
	Password string `json:"password"`
}

// handleAcceptInvitation redeems an invitation and creates the account.
//
// Unauthenticated by necessity: the person accepting has no account yet. The
// token is the whole authority, and it is deliberately narrow — it can create
// exactly the one account the invitation already names, with exactly the role
// the inviter chose.
func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req acceptInvitationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	presented := strings.TrimSpace(req.Token)

	v := newValidator()
	name := v.requireName("name", req.Name)
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

	kind, publicID, err := credential.Parse(presented)
	if err != nil || kind != credential.KindInvite {
		writeError(w, r, s.logger, ErrInvitationUnusable, nil)
		return
	}

	invitation, tokenHash, err := s.accounts.FindInvitationByPublicID(r.Context(), nil, publicID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrInvitationUnusable, nil)
			return
		}
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Compared in constant time, and compared before any state is inspected, so
	// that a wrong token and a used token take the same path.
	if !credential.Verify(presented, tokenHash) {
		s.recordRejectedInvitation(r, invitation.OrganizationID, invitation.ID, "verifier_mismatch")
		writeError(w, r, s.logger, ErrInvitationUnusable, nil)
		return
	}

	if !invitation.Pending(s.now()) {
		s.recordRejectedInvitation(r, invitation.OrganizationID, invitation.ID, invitation.Status(s.now()))
		writeError(w, r, s.logger, ErrInvitationUnusable, nil)
		return
	}

	passwordHash, err := credential.HashPassword(req.Password)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	var user *domain.User
	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		// Closing the invitation first means the unique index on the open row
		// does the arbitration: a second concurrent acceptance finds no open
		// row to update and rolls back before creating a duplicate account.
		var err error
		if user, err = s.accounts.CreateUser(r.Context(), tx, invitation.Email, name, passwordHash); err != nil {
			return err
		}
		if err = s.accounts.MarkInvitationAccepted(r.Context(), tx, invitation.ID, user.ID); err != nil {
			return err
		}

		// Email and role come from the invitation row, never from the request.
		// This is the property that stops a holder of a valid invitation from
		// joining as somebody else or promoting themselves to owner.
		if _, err = s.accounts.CreateMembership(r.Context(), tx, &domain.Membership{
			OrganizationID: invitation.OrganizationID,
			UserID:         user.ID,
			Role:           invitation.Role,
			CreatedBy:      invitation.CreatedBy,
			ExpiresAt:      invitation.MembershipExpiresAt,
		}); err != nil {
			return err
		}

		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: invitation.OrganizationID,
			Type:           audit.EventInvitationAccepted,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      domain.ActorHuman,
			ActorID:        &user.ID,
			ActorLabel:     invitation.Email,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata: map[string]any{
				"invitation_id": invitation.ID.String(),
				"role":          string(invitation.Role),
				"invited_by":    invitedByLabel(invitation),
			},
		})
	})
	if err != nil {
		// A lost race, or an address that acquired an account between the check
		// above and here. Neither is worth distinguishing to the caller.
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
			writeError(w, r, s.logger, ErrInvitationUnusable, nil)
			return
		}
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// No session is issued. Signing in immediately afterwards proves the
	// password was stored as typed, and it keeps this endpoint from being a
	// second way to mint a session.
	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"email": invitation.Email,
		"role":  string(invitation.Role),
	})
}

// recordRejectedInvitation notes a failed redemption.
//
// Worth recording: repeated failures against an organization's invitations is
// what a token being guessed or a leaked link being probed looks like.
func (s *Server) recordRejectedInvitation(r *http.Request, orgID, invitationID uuid.UUID, reason string) {
	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: orgID,
		Type:           audit.EventInvitationRejected,
		Outcome:        audit.OutcomeDenied,
		ActorType:      domain.ActorHuman,
		ActorLabel:     "anonymous",
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Metadata: map[string]any{
			"invitation_id": invitationID.String(),
			"reason":        reason,
		},
	})
}

func invitedByLabel(inv *store.Invitation) string {
	if inv.CreatedBy == nil {
		return ""
	}
	return inv.CreatedBy.String()
}
