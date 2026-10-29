package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

// handleListMemberships returns the organizations the caller may act in.
//
// Only ever the caller's own. There is no parameter here naming a user, so
// there is no version of this route that enumerates somebody else's
// memberships, which is the shape this endpoint would otherwise drift into.
func (s *Server) handleListMemberships(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	// Machine identities do not have memberships: a runtime token is issued
	// against one environment in one organization and cannot move.
	if principal.ActorType != domain.ActorHuman {
		writeError(w, r, s.logger, ErrForbidden, nil)
		return
	}

	memberships, err := s.accounts.ListMemberships(r.Context(), principal.ID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]map[string]any, 0, len(memberships))
	for _, m := range memberships {
		out = append(out, map[string]any{
			"organizationId": m.OrganizationID.String(),
			"name":           m.Name,
			"slug":           m.Slug,
			"role":           string(m.Role),
			"current":        m.OrganizationID == principal.OrganizationID,
			"expiresAt":      formatTimePtr(m.ExpiresAt),
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"organizations": out})
}

type switchOrganizationRequest struct {
	// Organization accepts a slug or an id, because a person types a slug and
	// an interface has an id, and refusing either would mean two endpoints.
	Organization string `json:"organization"`
}

// handleSwitchOrganization re-issues the caller's session against another
// organization they belong to.
//
// A session is bound to one organization, which is what keeps every query in
// the system scoped without threading an organization parameter through it.
// Switching therefore means minting a new session and revoking the old one,
// not mutating the row: a credential that silently changes what it authorizes
// is a credential nobody can reason about, and any copy of the old token would
// have quietly gained access to the new organization.
//
// Membership is re-checked here rather than trusted from the listing, and it is
// the only thing being checked: this grants no authority the caller did not
// already hold, it only decides which of their existing authorities is active.
func (s *Server) handleSwitchOrganization(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}
	if principal.ActorType != domain.ActorHuman {
		writeError(w, r, s.logger, ErrForbidden, nil)
		return
	}

	var req switchOrganizationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	wanted := strings.TrimSpace(req.Organization)
	if wanted == "" {
		v := newValidator()
		v.add("organization", "Name the organization to switch to.")
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	org, err := s.resolveOrganization(r, wanted)
	if err != nil {
		// A slug that does not exist and one the caller cannot enter answer
		// alike, so this endpoint cannot be used to discover which
		// organizations exist.
		writeError(w, r, s.logger, ErrNotFound, err)
		return
	}

	membership, err := s.accounts.FindMembershipInOrganization(r.Context(), principal.ID, org.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, s.logger, ErrNotFound, err)
			return
		}
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	if org.ID == principal.OrganizationID {
		// Already here. Answering with the current session rather than minting
		// a new one keeps `ward org use <current>` from silently invalidating
		// the token the caller is holding.
		writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
			"sessionToken":   nil,
			"unchanged":      true,
			"organizationId": org.ID.String(),
			"organization":   org.Name,
			"slug":           org.Slug,
			"role":           string(membership.Role),
		})
		return
	}

	switched, err := s.reissueSessionFor(r, principal, org.ID)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}
	clientIP := ClientIP(r, s.cfg.TrustProxyHeaders)

	// Recorded in both organizations. The one being left has as much reason to
	// know that a session moved as the one being entered.
	for _, orgID := range []uuid.UUID{principal.OrganizationID, org.ID} {
		s.audit.Record(r.Context(), audit.Event{
			OrganizationID: orgID,
			Type:           audit.EventOrganizationSwitched,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      domain.ActorHuman,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			IPAddress:      clientIP,
			UserAgent:      r.UserAgent(),
			Metadata: map[string]any{
				"from_organization_id": principal.OrganizationID.String(),
				"to_organization_id":   org.ID.String(),
				"session_kind":         principal.SessionKind,
			},
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"sessionToken":   switched.Token,
		"expiresAt":      switched.ExpiresAt.UTC().Format(time.RFC3339),
		"organizationId": org.ID.String(),
		"organization":   org.Name,
		"slug":           org.Slug,
		"role":           string(membership.Role),
	})
}

// resolveOrganization accepts either an id or a slug.
func (s *Server) resolveOrganization(r *http.Request, wanted string) (*domain.Organization, error) {
	if id, err := uuid.Parse(wanted); err == nil {
		return s.accounts.GetOrganization(r.Context(), id)
	}
	return s.accounts.GetOrganizationBySlug(r.Context(), strings.ToLower(wanted))
}

// reissuedSession is a replacement session bound to a different organization.
type reissuedSession struct {
	Token     string
	ExpiresAt time.Time
}

// reissueSessionFor mints a session bound to orgID and retires the caller's
// current one.
//
// A session is bound to one organization for the whole of its life, which is
// what keeps every query scoped without threading an organization through it.
// Moving therefore means a new credential, not a mutated row: a token that
// silently changed what it authorized could not be reasoned about, and any
// copy of the old one would have quietly gained the new access.
//
// Shared by switching between organizations and by founding one, because the
// session problem is identical in both and solving it twice is how the two
// drift apart.
func (s *Server) reissueSessionFor(
	r *http.Request,
	principal *domain.Principal,
	orgID uuid.UUID,
) (*reissuedSession, error) {
	ttl := s.cfg.SessionTTL
	if principal.SessionKind == "CLI" {
		ttl = s.cfg.CLISessionTTL
	}

	token, err := credential.Mint(credential.KindSession)
	if err != nil {
		return nil, err
	}

	session := &store.Session{
		UserID:         principal.ID,
		OrganizationID: orgID,
		Kind:           principal.SessionKind,
		ExpiresAt:      s.now().Add(ttl),
	}

	clientIP := ClientIP(r, s.cfg.TrustProxyHeaders)
	if err := s.accounts.CreateSession(r.Context(), session, token.PublicID, token.Hash, clientIP, r.UserAgent()); err != nil {
		return nil, err
	}

	// The old session goes only after the new one exists, so a failure here
	// leaves the caller signed in rather than signed out of everything.
	if err := s.accounts.RevokeSession(r.Context(), principal.CredentialID); err != nil {
		s.logger.Warn("could not revoke the previous session", "error", err)
	}

	return &reissuedSession{Token: token.Secret, ExpiresAt: session.ExpiresAt}, nil
}
