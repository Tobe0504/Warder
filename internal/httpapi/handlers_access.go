package httpapi

import (
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

// grantResponse is one row of the access screen.
//
// It reports capabilities individually rather than as a role name, because the
// question the screen has to answer is not "what is this person" but "can they
// use this, and can they see it" — two different answers that a role would
// collapse into one.
type grantResponse struct {
	ID          string `json:"id"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	SubjectName string `json:"subjectName"`
	SubjectKind string `json:"subjectKind"`

	Scope         string `json:"scope"`
	ProjectID     any    `json:"projectId"`
	EnvironmentID any    `json:"environmentId"`
	SecretID      any    `json:"secretId"`

	Capabilities []string `json:"capabilities"`
	CanUse       bool     `json:"canUse"`
	CanReveal    bool     `json:"canReveal"`

	CreatedAt string `json:"createdAt"`
	ExpiresAt any    `json:"expiresAt"`
	Reason    string `json:"reason"`
	Temporary bool   `json:"temporary"`
}

// handleListAccess returns the grants touching a project.
func (s *Server) handleListAccess(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{ProjectID: projectID}) {
		return
	}
	if _, err := s.projects.GetProject(r.Context(), principal.OrganizationID, projectID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	views, err := s.grants.ListForProject(r.Context(), principal.OrganizationID, projectID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]grantResponse, 0, len(views))
	for _, v := range views {
		capabilities := make([]string, 0, len(v.Capabilities))
		canUse, canReveal := false, false
		for _, c := range v.Capabilities {
			capabilities = append(capabilities, string(c))
			switch c {
			case domain.CapUseSecret:
				canUse = true
			case domain.CapReadSecret:
				canReveal = true
			}
		}

		out = append(out, grantResponse{
			ID:            v.ID.String(),
			SubjectType:   string(v.SubjectType),
			SubjectID:     v.SubjectID.String(),
			SubjectName:   v.SubjectName,
			SubjectKind:   v.SubjectKind,
			Scope:         describeScope(v.AccessGrant),
			ProjectID:     uuidString(v.ProjectID),
			EnvironmentID: uuidString(v.EnvironmentID),
			SecretID:      uuidString(v.SecretID),
			Capabilities:  capabilities,
			CanUse:        canUse,
			CanReveal:     canReveal,
			CreatedAt:     v.CreatedAt.UTC().Format(time.RFC3339),
			ExpiresAt:     formatTimePtr(v.ExpiresAt),
			Reason:        v.Reason,
			Temporary:     v.ExpiresAt != nil,
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"grants": out})
}

func describeScope(g domain.AccessGrant) string {
	switch {
	case g.SecretID != nil:
		return "secret"
	case g.EnvironmentID != nil:
		return "environment"
	case g.ProjectID != nil:
		return "project"
	default:
		return "organization"
	}
}

func uuidString(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

type grantAccessRequest struct {
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`

	// EnvironmentID and SecretID narrow the grant. Leaving a level unset is not
	// how a wildcard is requested; see AllEnvironments below.
	EnvironmentID string `json:"environmentId"`
	SecretID      string `json:"secretId"`

	// AllEnvironments must be set deliberately to grant across a whole project.
	// A wildcard that could be produced by omitting a field is a wildcard that
	// gets created by a form bug.
	AllEnvironments bool `json:"allEnvironments"`

	Capabilities []string `json:"capabilities"`
	ExpiresAt    string   `json:"expiresAt"`
	Reason       string   `json:"reason"`
}

// handleGrantAccess creates an access grant.
//
// This is the endpoint that makes "use without seeing" real, and the one an
// auditor will look at first. Granting requires MANAGE_ACCESS, every grant is
// audited with its capabilities and expiry, and a grant conferring READ_SECRET
// is recorded with a reason, because the interesting question about plaintext
// visibility is why rather than who.
func (s *Server) handleGrantAccess(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageAccess, authzTarget{ProjectID: projectID}) {
		return
	}
	if _, err := s.projects.GetProject(r.Context(), principal.OrganizationID, projectID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	var req grantAccessRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	subjectType := domain.SubjectType(strings.ToUpper(strings.TrimSpace(req.SubjectType)))
	if !domain.ValidSubjectType(subjectType) {
		v.add("subjectType", "Choose USER or MACHINE.")
	}
	subjectID := v.requireUUID("subjectId", req.SubjectID)
	capabilities := v.capabilities("capabilities", req.Capabilities)
	expiresAt := v.futureTime("expiresAt", req.ExpiresAt, s.now())
	reason := v.optionalText("reason", req.Reason, maxReasonLength)

	var environmentID, secretID *uuid.UUID
	if !req.AllEnvironments {
		parsed := v.requireUUID("environmentId", req.EnvironmentID)
		if parsed != uuid.Nil {
			environmentID = &parsed
		}
	}
	if strings.TrimSpace(req.SecretID) != "" {
		if req.AllEnvironments {
			v.add("secretId", "A secret-scoped grant must name an environment.")
		}
		parsed := v.requireUUID("secretId", req.SecretID)
		if parsed != uuid.Nil {
			secretID = &parsed
		}
	}
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	// Verify the subject exists in this organization. Without this a grant
	// could be written against an identifier from another tenant, which would
	// later resolve if that identifier were ever reused.
	subjectName, err := s.resolveSubject(r, principal.OrganizationID, subjectType, subjectID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Verify the environment belongs to this project.
	if environmentID != nil {
		environment, err := s.projects.GetEnvironment(r.Context(), principal.OrganizationID, *environmentID)
		if err != nil {
			writeError(w, r, s.logger, translateError(err), err)
			return
		}
		if environment.ProjectID != projectID {
			writeError(w, r, s.logger, Validation(map[string]string{
				"environmentId": "That environment is not part of this project.",
			}), nil)
			return
		}
	}

	grant := &domain.AccessGrant{
		OrganizationID: principal.OrganizationID,
		SubjectType:    subjectType,
		SubjectID:      subjectID,
		ProjectID:      &projectID,
		EnvironmentID:  environmentID,
		SecretID:       secretID,
		Capabilities:   capabilities,
		CreatedBy:      principal.ID,
		ExpiresAt:      expiresAt,
		Reason:         reason,
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.grants.Create(r.Context(), tx, grant); err != nil {
			return err
		}

		capabilityNames := make([]string, 0, len(capabilities))
		grantsPlaintext := false
		for _, c := range capabilities {
			capabilityNames = append(capabilityNames, string(c))
			if c == domain.CapReadSecret {
				grantsPlaintext = true
			}
		}

		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventAccessGranted,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			ProjectID:      &projectID,
			EnvironmentID:  environmentID,
			SecretID:       secretID,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Reason:         reason,
			Metadata: map[string]any{
				"grant_id":     grant.ID.String(),
				"subject_type": string(subjectType),
				"subject_id":   subjectID.String(),
				"subject_name": subjectName,
				"capabilities": capabilityNames,
				"expires":      expiresAt != nil,
				// Flagged so that a search for plaintext-visibility changes is a
				// single filter rather than a scan of every grant event.
				"grants_plaintext_visibility": grantsPlaintext,
				"self_granted":                subjectType == domain.SubjectUser && subjectID == principal.ID,
			},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{"id": grant.ID.String()})
}

func (s *Server) resolveSubject(r *http.Request, orgID uuid.UUID, subjectType domain.SubjectType, subjectID uuid.UUID) (string, error) {
	if subjectType == domain.SubjectMachine {
		identity, err := s.machines.GetIdentity(r.Context(), orgID, subjectID)
		if err != nil {
			return "", err
		}
		return identity.Name, nil
	}

	members, err := s.accounts.ListMembers(r.Context(), orgID)
	if err != nil {
		return "", err
	}
	for _, m := range members {
		if m.UserID == subjectID {
			return m.Name, nil
		}
	}
	return "", store.ErrNotFound
}

// handleRevokeAccess ends a grant. Runtime access stops on the next request.
func (s *Server) handleRevokeAccess(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, projectValid := pathUUID(r, "projectID")
	grantID, grantValid := pathUUID(r, "grantID")
	if !projectValid || !grantValid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageAccess, authzTarget{ProjectID: projectID}) {
		return
	}

	grant, err := s.grants.Get(r.Context(), principal.OrganizationID, grantID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.grants.Revoke(r.Context(), tx, principal.OrganizationID, grantID); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventAccessRevoked,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			ProjectID:      grant.ProjectID,
			EnvironmentID:  grant.EnvironmentID,
			SecretID:       grant.SecretID,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata: map[string]any{
				"grant_id":   grantID.String(),
				"subject_id": grant.SubjectID.String(),
			},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Machine identities
// ---------------------------------------------------------------------------

type createIdentityRequest struct {
	Name      string `json:"name"`
	ActorType string `json:"actorType"`
	// ExpiresAt bounds the identity itself, which is how an AI agent session is
	// made temporary by construction rather than by remembering to clean up.
	ExpiresAt string `json:"expiresAt"`
}

// handleCreateIdentity creates a machine identity.
func (s *Server) handleCreateIdentity(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !s.allow(w, r, principal, domain.CapManageAccess, authzTarget{}) {
		return
	}

	var req createIdentityRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	name := v.requireName("name", req.Name)
	expiresAt := v.futureTime("expiresAt", req.ExpiresAt, s.now())

	actorType := domain.ActorType(strings.ToUpper(strings.TrimSpace(req.ActorType)))
	validActor := false
	for _, a := range domain.MachineActorTypes {
		if a == actorType {
			validActor = true
		}
	}
	if !validActor {
		v.add("actorType", "Choose SERVICE, AI_AGENT, CI, or WORKLOAD.")
	}
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	identity := &domain.MachineIdentity{
		OrganizationID: principal.OrganizationID,
		Name:           name,
		ActorType:      actorType,
		CreatedBy:      principal.ID,
		ExpiresAt:      expiresAt,
	}

	err := store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.machines.CreateIdentity(r.Context(), tx, identity); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventIdentityCreated,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata: map[string]any{
				"identity_id":   identity.ID.String(),
				"identity_name": name,
				"actor_type":    string(actorType),
				"expires":       expiresAt != nil,
			},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"id":        identity.ID.String(),
		"name":      identity.Name,
		"actorType": string(identity.ActorType),
		"expiresAt": formatTimePtr(identity.ExpiresAt),
	})
}

// handleDisableIdentity permanently stops an identity from authenticating.
//
// This is the offboarding action for a machine: an agent session that is over,
// a service being decommissioned, a CI pipeline that has been retired. It is
// one action rather than a hunt through that identity's tokens, and it takes
// effect on the next request — including for sessions already issued.
//
// There is deliberately no matching re-enable. Bringing a disabled identity
// back would silently restore whatever grants it still held, which is the kind
// of thing that gets clicked to fix an outage and then forgotten. Creating a
// fresh identity is the same amount of work and leaves a record of the
// decision.
func (s *Server) handleDisableIdentity(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !s.allow(w, r, principal, domain.CapManageAccess, authzTarget{}) {
		return
	}

	identityID, valid := pathUUID(r, "identityID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	// Resolved first so the audit record can name it after it is gone, and so
	// an identity in another organization reports as absent.
	identity, err := s.machines.GetIdentity(r.Context(), principal.OrganizationID, identityID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	if err := s.machines.DisableIdentity(r.Context(), principal.OrganizationID, identityID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: principal.OrganizationID,
		Type:           audit.EventIdentityDisabled,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      principal.ActorType,
		ActorID:        &principal.ID,
		ActorLabel:     principal.DisplayName,
		CredentialID:   &principal.CredentialID,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Metadata: map[string]any{
			"identity_id":              identityID.String(),
			"identity_name":            identity.Name,
			"actor_type":               string(identity.ActorType),
			"derived_sessions_revoked": true,
		},
	})

	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}

// handleListIdentities returns the organization's machine identities.
func (s *Server) handleListIdentities(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{}) {
		return
	}

	identities, err := s.machines.ListIdentities(r.Context(), principal.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	now := s.now()
	out := make([]map[string]any, 0, len(identities))
	for _, i := range identities {
		out = append(out, map[string]any{
			"id":        i.ID.String(),
			"name":      i.Name,
			"actorType": string(i.ActorType),
			"createdAt": i.CreatedAt.UTC().Format(time.RFC3339),
			"expiresAt": formatTimePtr(i.ExpiresAt),
			"active":    i.Active(now),
		})
	}
	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"identities": out})
}

// ---------------------------------------------------------------------------
// Machine tokens
// ---------------------------------------------------------------------------

type createTokenRequest struct {
	IdentityID    string   `json:"identityId"`
	Name          string   `json:"name"`
	EnvironmentID string   `json:"environmentId"`
	Capabilities  []string `json:"capabilities"`
	SecretKeys    []string `json:"secretKeys"`
	ExpiresAt     string   `json:"expiresAt"`
}

// handleCreateToken mints a scoped runtime token.
//
// The full credential appears in this response and is never retrievable again:
// only its verifier is stored. That is why the response says so explicitly —
// a person who closes the dialog without copying it needs to know they must
// mint a new one rather than go looking for it.
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageAccess, authzTarget{ProjectID: projectID}) {
		return
	}

	var req createTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	name := v.requireName("name", req.Name)
	identityID := v.requireUUID("identityId", req.IdentityID)
	environmentID := v.requireUUID("environmentId", req.EnvironmentID)
	capabilities := v.capabilities("capabilities", req.Capabilities)
	expiresAt := v.futureTime("expiresAt", req.ExpiresAt, s.now())

	secretKeys := make([]string, 0, len(req.SecretKeys))
	for _, key := range req.SecretKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if !validSecretKey(trimmed) {
			v.add("secretKeys", "Secret keys may contain only letters, digits, and underscores.")
			break
		}
		secretKeys = append(secretKeys, trimmed)
	}
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	identity, err := s.machines.GetIdentity(r.Context(), principal.OrganizationID, identityID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	environment, err := s.projects.GetEnvironment(r.Context(), principal.OrganizationID, environmentID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if environment.ProjectID != projectID {
		writeError(w, r, s.logger, Validation(map[string]string{
			"environmentId": "That environment is not part of this project.",
		}), nil)
		return
	}

	token, err := credential.Mint(credential.KindMachine)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	record := &store.MachineToken{
		MachineIdentityID: identityID,
		OrganizationID:    principal.OrganizationID,
		Name:              name,
		ProjectID:         projectID,
		EnvironmentID:     environmentID,
		Capabilities:      capabilities,
		SecretKeys:        secretKeys,
		PublicID:          token.PublicID,
		ExpiresAt:         expiresAt,
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.machines.CreateToken(r.Context(), tx, record, token.Hash, &principal.ID); err != nil {
			return err
		}

		capabilityNames := make([]string, 0, len(capabilities))
		for _, c := range capabilities {
			capabilityNames = append(capabilityNames, string(c))
		}

		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventTokenCreated,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			ProjectID:      &projectID,
			EnvironmentID:  &environmentID,
			TokenID:        &record.ID,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata: map[string]any{
				"token_name":    name,
				"token_prefix":  token.PublicID,
				"identity_name": identity.Name,
				"actor_type":    string(identity.ActorType),
				"capabilities":  capabilityNames,
				"secret_keys":   secretKeys,
				"expires":       expiresAt != nil,
			},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"id":     record.ID.String(),
		"token":  token.Secret,
		"prefix": token.PublicID,
		"notice": "This token is shown once and cannot be retrieved again. Store it somewhere your runtime can read it.",
	})
}

// handleListTokens returns a project's tokens, never their secrets.
func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{ProjectID: projectID}) {
		return
	}
	// Resolved explicitly, so a foreign project answers 404 like every other
	// endpoint rather than an empty 200. See handleListEnvironments.
	if _, err := s.projects.GetProject(r.Context(), principal.OrganizationID, projectID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	tokens, err := s.machines.ListTokens(r.Context(), principal.OrganizationID, projectID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	now := s.now()
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		capabilities := make([]string, 0, len(t.Capabilities))
		for _, c := range t.Capabilities {
			capabilities = append(capabilities, string(c))
		}
		out = append(out, map[string]any{
			"id":            t.ID.String(),
			"name":          t.Name,
			"display":       credential.Display(credential.KindMachine, t.PublicID),
			"prefix":        t.PublicID,
			"identityName":  t.IdentityName,
			"actorType":     string(t.IdentityActorType),
			"environmentId": t.EnvironmentID.String(),
			"capabilities":  capabilities,
			"secretKeys":    t.SecretKeys,
			"createdAt":     t.CreatedAt.UTC().Format(time.RFC3339),
			"expiresAt":     formatTimePtr(t.ExpiresAt),
			"revokedAt":     formatTimePtr(t.RevokedAt),
			"lastUsedAt":    formatTimePtr(t.LastUsedAt),
			"active":        t.Active(now),
		})
	}
	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"tokens": out})
}

// handleRevokeToken revokes a token and every runtime session derived from it.
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	tokenID, valid := pathUUID(r, "tokenID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageAccess, authzTarget{}) {
		return
	}

	if err := s.machines.RevokeToken(r.Context(), principal.OrganizationID, tokenID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: principal.OrganizationID,
		Type:           audit.EventTokenRevoked,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      principal.ActorType,
		ActorID:        &principal.ID,
		ActorLabel:     principal.DisplayName,
		CredentialID:   &principal.CredentialID,
		TokenID:        &tokenID,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Metadata:       map[string]any{"derived_sessions_revoked": true},
	})

	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}
