package httpapi

import (
	"net/http"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/secretvalue"
	"github.com/Tobe0504/Warder/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// maskedValue is what the dashboard receives in place of every secret value.
//
// It is a constant, not a length-preserving mask. Rendering one dot per
// character would publish the length of every credential in the organization to
// anyone who can list secrets, which narrows a guess and distinguishes a
// 32-character API key from a 64-character one.
const maskedValue = "••••••••"

// secretResponse is the metadata view of a secret.
//
// There is no value field, and there is no code path that adds one. Revealing a
// value is a separate, explicitly requested, separately authorized, separately
// rate-limited, separately audited endpoint.
type secretResponse struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Description string `json:"description"`
	// Masked is what the interface displays. Sending a placeholder rather than
	// omitting the field keeps the UI honest: there is a value, and you are not
	// being shown it.
	Masked string `json:"masked"`

	Version         *int   `json:"version,omitempty"`
	Status          string `json:"status"`
	ExpiresAt       any    `json:"expiresAt"`
	LastUsedAt      any    `json:"lastUsedAt"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	EncryptionKeyID string `json:"encryptionKeyId,omitempty"`

	// Capabilities the requesting user holds over this secret, so the interface
	// can show a reveal control only to someone who could actually use it —
	// derived from the same engine that enforces the decision.
	CanUse    bool `json:"canUse"`
	CanReveal bool `json:"canReveal"`
	CanRotate bool `json:"canRotate"`
}

func (s *Server) toSecretResponse(summary store.SecretSummary, caps secretCapabilities) secretResponse {
	status := "NO_VERSION"
	if summary.VersionStatus != nil {
		status = string(*summary.VersionStatus)
	}
	if summary.Expired(s.now()) {
		status = "EXPIRED"
	}

	out := secretResponse{
		ID:          summary.ID.String(),
		Key:         summary.Key,
		Description: summary.Description,
		Masked:      maskedValue,
		Version:     summary.Version,
		Status:      status,
		ExpiresAt:   formatTimePtr(summary.VersionExpires),
		LastUsedAt:  formatTimePtr(summary.LastUsedAt),
		CreatedAt:   summary.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   summary.UpdatedAt.UTC().Format(time.RFC3339),
		CanUse:      caps.use,
		CanReveal:   caps.reveal,
		CanRotate:   caps.rotate,
	}
	if summary.EncryptionKeyID != nil {
		out.EncryptionKeyID = *summary.EncryptionKeyID
	}
	return out
}

type secretCapabilities struct {
	use    bool
	reveal bool
	rotate bool
}

// handleListSecrets returns secret metadata for an environment.
func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	environmentID, valid := pathUUID(r, "environmentID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	environment, err := s.projects.GetEnvironment(r.Context(), principal.OrganizationID, environmentID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	target := authzTarget{ProjectID: environment.ProjectID, EnvironmentID: environmentID}
	if !s.allow(w, r, principal, domain.CapReadMetadata, target) {
		return
	}

	summaries, err := s.secretsRepo().ListSecrets(r.Context(), principal.OrganizationID, environmentID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]secretResponse, 0, len(summaries))
	for _, summary := range summaries {
		caps := s.capabilitiesFor(r, principal, environment.ProjectID, environmentID, summary)
		out = append(out, s.toSecretResponse(summary, caps))
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"environment": toEnvironmentResponse(*environment),
		"secrets":     out,
	})
}

// capabilitiesFor asks the policy engine what this user holds over a secret.
//
// The interface must not decide for itself who may reveal a value. Asking the
// engine means the control the user sees and the decision the server makes come
// from the same rules, and cannot drift apart.
func (s *Server) capabilitiesFor(r *http.Request, p *domain.Principal, projectID, environmentID uuid.UUID, summary store.SecretSummary) secretCapabilities {
	check := func(c domain.Capability) bool {
		decision, err := s.policy.Authorize(r.Context(), authzRequestFor(p, c, projectID, environmentID, summary))
		if err != nil {
			// If the engine cannot answer, the interface shows the control as
			// unavailable. Enforcement will refuse anyway; this only avoids
			// offering a button that would fail.
			s.logger.Error("could not evaluate capability for display", "error", err)
			return false
		}
		return decision.Allowed
	}

	return secretCapabilities{
		use:    check(domain.CapUseSecret),
		reveal: check(domain.CapReadSecret),
		rotate: check(domain.CapRotateSecret),
	}
}

type createSecretRequest struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Value       string `json:"value"`
	ExpiresAt   string `json:"expiresAt"`
}

// handleCreateSecret stores a new secret.
func (s *Server) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	environmentID, valid := pathUUID(r, "environmentID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	environment, err := s.projects.GetEnvironment(r.Context(), principal.OrganizationID, environmentID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	var req createSecretRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	key := v.requireSecretKey("key", req.Key)
	description := v.optionalText("description", req.Description, maxDescriptionLength)
	expiresAt := v.futureTime("expiresAt", req.ExpiresAt, s.now())
	if req.Value == "" {
		v.add("value", "A value is required.")
	}
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	// The plaintext is wrapped immediately on arrival and stays wrapped until
	// the encryption service opens it. From this line on it cannot be logged or
	// serialized by accident.
	value := secretvalue.NewString(req.Value)
	defer value.Destroy()

	secret, version, err := s.secrets.Create(r.Context(), s.requestContext(r, principal), secrets.CreateRequest{
		OrganizationID: principal.OrganizationID,
		ProjectID:      environment.ProjectID,
		EnvironmentID:  environmentID,
		Key:            key,
		Description:    description,
		Value:          value,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"id":      secret.ID.String(),
		"key":     secret.Key,
		"version": version.Version,
		"masked":  maskedValue,
	})
}

type rotateSecretRequest struct {
	Value     string `json:"value"`
	ExpiresAt string `json:"expiresAt"`
}

// handleRotateSecret stores a new version and activates it.
func (s *Server) handleRotateSecret(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	secretID, valid := pathUUID(r, "secretID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	summary, environment, err := s.locateSecret(r, principal, secretID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	var req rotateSecretRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	expiresAt := v.futureTime("expiresAt", req.ExpiresAt, s.now())
	if req.Value == "" {
		v.add("value", "A value is required.")
	}
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	value := secretvalue.NewString(req.Value)
	defer value.Destroy()

	version, err := s.secrets.Rotate(r.Context(), s.requestContext(r, principal), secrets.RotateRequest{
		OrganizationID: principal.OrganizationID,
		ProjectID:      environment.ProjectID,
		EnvironmentID:  environment.ID,
		SecretID:       summary.ID,
		Value:          value,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"version": version.Version,
		"status":  string(version.Status),
		"masked":  maskedValue,
		// Rotation replaces what Warder stores. Whether the credential itself
		// was rotated at the provider is a separate act, and the response says
		// so rather than letting an operator assume otherwise.
		"upstreamRotated": false,
	})
}

// handleRevealSecret returns a plaintext value to an authorized human.
func (s *Server) handleRevealSecret(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	secretID, valid := pathUUID(r, "secretID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	_, environment, err := s.locateSecret(r, principal, secretID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// Authorization, audit, and decryption all happen inside the service, in
	// that order.
	value, err := s.secrets.Reveal(r.Context(), s.requestContext(r, principal),
		principal.OrganizationID, environment.ProjectID, environment.ID, secretID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	defer value.Destroy()

	// One of the few deliberate crossings. Expose is called here, once,
	// immediately before the response is written.
	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"value": value.ExposeString(),
	})
}

// handleListVersions returns version history without any material.
func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	secretID, valid := pathUUID(r, "secretID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	summary, environment, err := s.locateSecret(r, principal, secretID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{
		ProjectID: environment.ProjectID, EnvironmentID: environment.ID,
		SecretID: secretID, SecretKey: summary.Key,
	}) {
		return
	}

	versions, err := s.secretsRepo().ListVersions(r.Context(), principal.OrganizationID, secretID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	now := s.now()
	out := make([]map[string]any, 0, len(versions))
	for _, v := range versions {
		out = append(out, map[string]any{
			"id":              v.ID.String(),
			"version":         v.Version,
			"status":          string(v.Status),
			"createdAt":       v.CreatedAt.UTC().Format(time.RFC3339),
			"expiresAt":       formatTimePtr(v.ExpiresAt),
			"revokedAt":       formatTimePtr(v.RevokedAt),
			"expired":         v.Expired(now),
			"deliverable":     v.Deliverable(now),
			"encryptionKeyId": v.EncryptionKeyID,
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{
		"secret":   map[string]any{"id": summary.ID.String(), "key": summary.Key},
		"versions": out,
	})
}

type versionActionRequest struct {
	VersionID string `json:"versionId"`
}

// handleRevokeVersion marks a version as never deliverable again.
func (s *Server) handleRevokeVersion(w http.ResponseWriter, r *http.Request) {
	s.versionAction(w, r, domain.CapRevokeSecret, audit.EventSecretRevoked,
		func(ctx contextArgs) error {
			return s.secretsRepo().RevokeVersion(ctx.request.Context(), ctx.tx, ctx.secretID, ctx.versionID)
		})
}

// handleRollbackVersion makes an earlier version active again.
func (s *Server) handleRollbackVersion(w http.ResponseWriter, r *http.Request) {
	s.versionAction(w, r, domain.CapRotateSecret, audit.EventSecretRolledBack,
		func(ctx contextArgs) error {
			return s.secretsRepo().ActivateVersion(ctx.request.Context(), ctx.tx, ctx.secretID, ctx.versionID)
		})
}

type setExpiryRequest struct {
	VersionID string `json:"versionId"`
	// ExpiresAt empty clears the expiry.
	ExpiresAt string `json:"expiresAt"`
}

// handleSetExpiry sets or clears a version's expiry.
func (s *Server) handleSetExpiry(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	secretID, valid := pathUUID(r, "secretID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	summary, environment, err := s.locateSecret(r, principal, secretID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if !s.allow(w, r, principal, domain.CapRotateSecret, authzTarget{
		ProjectID: environment.ProjectID, EnvironmentID: environment.ID,
		SecretID: secretID, SecretKey: summary.Key,
	}) {
		return
	}

	var req setExpiryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	versionID := v.requireUUID("versionId", req.VersionID)
	expiresAt := v.futureTime("expiresAt", req.ExpiresAt, s.now())
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := s.secretsRepo().SetVersionExpiry(r.Context(), tx, secretID, versionID, expiresAt); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventSecretExpiryChanged,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			ProjectID:      &environment.ProjectID,
			EnvironmentID:  &environment.ID,
			SecretID:       &secretID,
			SecretKey:      summary.Key,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata: map[string]any{
				"version_id": versionID.String(),
				"cleared":    expiresAt == nil,
			},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}

// contextArgs carries the pieces a version action needs.
type contextArgs struct {
	request   *http.Request
	tx        pgx.Tx
	secretID  uuid.UUID
	versionID uuid.UUID
}

// versionAction is the shared shape of revoke and rollback: locate the secret,
// authorize, act and audit in one transaction.
func (s *Server) versionAction(w http.ResponseWriter, r *http.Request, capability domain.Capability, eventType audit.EventType, act func(contextArgs) error) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	secretID, valid := pathUUID(r, "secretID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	summary, environment, err := s.locateSecret(r, principal, secretID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	if !s.allow(w, r, principal, capability, authzTarget{
		ProjectID: environment.ProjectID, EnvironmentID: environment.ID,
		SecretID: secretID, SecretKey: summary.Key,
	}) {
		return
	}

	var req versionActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	versionID := v.requireUUID("versionId", req.VersionID)
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	err = store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := act(contextArgs{request: r, tx: tx, secretID: secretID, versionID: versionID}); err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           eventType,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			ProjectID:      &environment.ProjectID,
			EnvironmentID:  &environment.ID,
			SecretID:       &secretID,
			SecretKey:      summary.Key,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata:       map[string]any{"version_id": versionID.String()},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]bool{"ok": true})
}

// locateSecret resolves a secret and its environment within the caller's
// organization. A secret in another tenant reports as not found.
func (s *Server) locateSecret(r *http.Request, p *domain.Principal, secretID uuid.UUID) (*store.SecretSummary, *domain.Environment, error) {
	summary, err := s.secretsRepo().GetSecret(r.Context(), p.OrganizationID, secretID)
	if err != nil {
		return nil, nil, err
	}
	environment, err := s.projects.GetEnvironment(r.Context(), p.OrganizationID, summary.EnvironmentID)
	if err != nil {
		return nil, nil, err
	}
	return summary, environment, nil
}
