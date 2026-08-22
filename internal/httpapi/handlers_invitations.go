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

	// Scoped to this organization, not to Warder as a whole. Someone who
	// already has an account is exactly who a second organization wants to
	// invite; they accept it signed in, and it adds a membership rather than
	// an account. The only address with nothing to gain from an invitation is
	// one already in this organization.
	member, err := s.accounts.EmailBelongsToOrganization(r.Context(), principal.OrganizationID, email)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if member {
		writeError(w, r, s.logger, ErrAlreadyMember, nil)
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

// openInvitation resolves a presented token to a pending invitation.
//
// Shared by inspect and accept so that both answer identically for a token that
// is unknown, forged, expired, withdrawn, or already used. Two routes reaching
// the same conclusion by different code is how one of them eventually becomes
// an oracle the other is not.
func (s *Server) openInvitation(w http.ResponseWriter, r *http.Request, presented string) (*store.Invitation, bool) {
	kind, publicID, err := credential.Parse(presented)
	if err != nil || kind != credential.KindInvite {
		writeError(w, r, s.logger, ErrInvitationUnusable, nil)
		return nil, false
	}

	invitation, tokenHash, err := s.accounts.FindInvitationByPublicID(r.Context(), nil, publicID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrInvitationUnusable, nil)
			return nil, false
		}
		writeError(w, r, s.logger, translateError(err), err)
		return nil, false
	}

	// Compared in constant time, and compared before any state is inspected, so
	// that a wrong token and a used token take the same path.
	if !credential.Verify(presented, tokenHash) {
		s.recordRejectedInvitation(r, invitation.OrganizationID, invitation.ID, "verifier_mismatch")
		writeError(w, r, s.logger, ErrInvitationUnusable, nil)
		return nil, false
	}

	if !invitation.Pending(s.now()) {
		s.recordRejectedInvitation(r, invitation.OrganizationID, invitation.ID, invitation.Status(s.now()))
		writeError(w, r, s.logger, ErrInvitationUnusable, nil)
		return nil, false
	}

	return invitation, true
}

type inspectInvitationRequest struct {
	Token string `json:"token"`
}

// handleInspectInvitation reports what an invitation is for and what the person
// holding it has to do next.
//
// This exists so the interface can ask the right question instead of the
// average one. Without it the only way to discover that an address already has
// an account is to fill in a name and a password and have them rejected, which
// asks somebody to choose a credential they neither need nor will ever use.
//
// It discloses the organization, the role, the invited address, and whether
// that address has an account. All of it to a caller already holding a valid,
// pending invitation naming that address, sent to them by an administrator of
// that organization. An invalid token learns nothing: the answer is the same
// ErrInvitationUnusable every other invitation route gives.
func (s *Server) handleInspectInvitation(w http.ResponseWriter, r *http.Request) {
	var req inspectInvitationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	invitation, ok := s.openInvitation(w, r, strings.TrimSpace(req.Token))
	if !ok {
		return
	}

	org, err := s.accounts.GetOrganization(r.Context(), invitation.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	_, err = s.accounts.GetUserByEmail(r.Context(), invitation.Email)
	accountExists := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	// canClaim is the whole point: it answers "can this browser accept right
	// now", which is the only question the interface actually has.
	canClaim := false
	if principal, signedIn := PrincipalFrom(r.Context()); signedIn && principal.ActorType == domain.ActorHuman {
		if user, err := s.accounts.GetUser(r.Context(), principal.ID); err == nil {
			canClaim = strings.EqualFold(user.Email, invitation.Email)
		}
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"organization":  org.Name,
		"role":          string(invitation.Role),
		"email":         invitation.Email,
		"accountExists": accountExists,
		"canClaim":      canClaim,
	})
}

type acceptInvitationRequest struct {
	Token string `json:"token"`
	// Name is the invitee's own. The inviter's suggestion is only a default:
	// people should control how their name is written.
	Name     string `json:"name"`
	Password string `json:"password"`
}

// handleAcceptInvitation redeems an invitation and creates the account.
//
// Unauthenticated by necessity: the person accepting has no account yet. The
// token is the whole authority, and it is deliberately narrow; it can create
// exactly the one account the invitation already names, with exactly the role
// the inviter chose.
func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req acceptInvitationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	presented := strings.TrimSpace(req.Token)

	// Someone who is already signed in is claiming this invitation for the
	// account they are signed in as, so there is no account to create and
	// nothing to name or set a password for. Everything below that validates
	// those fields is skipped.
	claimant, claiming := PrincipalFrom(r.Context())
	if claiming && claimant.ActorType != domain.ActorHuman {
		// A machine token has no person behind it to join an organization.
		writeError(w, r, s.logger, ErrForbidden, nil)
		return
	}

	var name string
	if !claiming {
		v := newValidator()
		name = v.requireName("name", req.Name)
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
	}

	invitation, ok := s.openInvitation(w, r, presented)
	if !ok {
		return
	}

	// An invitation names an address, and that address is the whole point of it:
	// it is what stops a token holder from joining as somebody else. A signed-in
	// claimant must therefore be that address, or this is one person redeeming
	// another person's invitation.
	if claiming {
		claimantUser, err := s.accounts.GetUser(r.Context(), claimant.ID)
		if err != nil {
			writeError(w, r, s.logger, translateError(err), err)
			return
		}
		if !strings.EqualFold(claimantUser.Email, invitation.Email) {
			s.recordRejectedInvitation(r, invitation.OrganizationID, invitation.ID, "claimant_mismatch")
			writeError(w, r, s.logger, ErrInvitationUnusable, nil)
			return
		}
	}

	// Not signed in, but the address already has an account. The invitation is
	// still good; it just cannot be redeemed by creating a second account for
	// the same address, which is what the unique index used to refuse with a
	// misleading "this invitation is not valid".
	if !claiming {
		if _, err := s.accounts.GetUserByEmail(r.Context(), invitation.Email); err == nil {
			writeError(w, r, s.logger, ErrAccountExists, nil)
			return
		} else if !errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrInternal, err)
			return
		}
	}

	var passwordHash string
	if !claiming {
		var err error
		if passwordHash, err = credential.HashPassword(req.Password); err != nil {
			writeError(w, r, s.logger, ErrInternal, err)
			return
		}
	}

	var user *domain.User
	err := store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		// Closing the invitation first means the unique index on the open row
		// does the arbitration: a second concurrent acceptance finds no open
		// row to update and rolls back before creating a duplicate account.
		var err error
		if claiming {
			if user, err = s.accounts.GetUser(r.Context(), claimant.ID); err != nil {
				return err
			}
		} else if user, err = s.accounts.CreateUser(r.Context(), tx, invitation.Email, name, passwordHash); err != nil {
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
