package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/store"
)

// The runtime surface is the machine-to-machine API. It is served on its own
// listener, so it can be placed on a network the dashboard's backend cannot
// reach, and the BFF exposes no route that forwards to it.
//
// Its two endpoints have a deliberate division of labour:
//
//	POST /runtime/auth      exchanges a long-lived token for a short-lived,
//	                        narrowly scoped session, once per process start.
//	POST /runtime/secrets   delivers values, authenticated by that session.
//
// The long-lived credential therefore crosses the network rarely, and the one
// that accompanies actual secret delivery expires in minutes.

// runtimeAuthRequest is the exchange request.
//
// Project and environment are meaningful only for a human's CLI login, which
// carries no scope of its own and therefore has to say what it wants to run
// against. A machine token already names exactly one project and one
// environment; when one is presented, these fields are checked against the
// token's scope rather than used to widen it, so naming a different environment
// is refused instead of silently ignored.
type runtimeAuthRequest struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
}

type runtimeAuthResponse struct {
	// AccessToken is the short-lived runtime session.
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`

	// The resolved scope, so the CLI can tell the developer what it is about to
	// run against without having to guess.
	Project      string   `json:"project"`
	Environment  string   `json:"environment"`
	Identity     string   `json:"identity"`
	ActorType    string   `json:"actorType"`
	Capabilities []string `json:"capabilities"`
}

// handleRuntimeAuth exchanges a presented credential for a runtime session.
func (s *Server) handleRuntimeAuth(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	var req runtimeAuthRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, r, s.logger, ErrBadRequest, err)
			return
		}
	}

	rc := s.requestContext(r, principal)

	scope, apiErr, err := s.resolveRuntimeScope(r, principal, req)
	if apiErr != nil || err != nil {
		writeError(w, r, s.logger, apiErr, err)
		return
	}

	project, err := s.projects.GetProject(r.Context(), principal.OrganizationID, scope.ProjectID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	environment, err := s.projects.GetEnvironment(r.Context(), principal.OrganizationID, scope.EnvironmentID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	token, err := credential.Mint(credential.KindRuntime)
	if err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	subjectType := domain.SubjectMachine
	if principal.Type == domain.PrincipalUser {
		subjectType = domain.SubjectUser
	}

	session := &store.RuntimeSession{
		OrganizationID: principal.OrganizationID,
		SubjectType:    subjectType,
		SubjectID:      principal.ID,
		ActorType:      principal.ActorType,
		ProjectID:      scope.ProjectID,
		EnvironmentID:  scope.EnvironmentID,
		// The session inherits the resolved ceiling exactly. An exchange must
		// never be a way to acquire more than was presented.
		Capabilities:       scope.Capabilities,
		SecretKeys:         scope.SecretKeys,
		SourceCredentialID: &principal.CredentialID,
		PublicID:           token.PublicID,
		ExpiresAt:          s.now().Add(s.runtimeSessionTTL),
	}

	if err := s.machines.CreateRuntimeSession(r.Context(), session, token.Hash); err != nil {
		writeError(w, r, s.logger, ErrInternal, err)
		return
	}

	if err := s.machines.TouchToken(r.Context(), principal.CredentialID); err != nil {
		s.logger.Warn("could not record token use", "error", err)
	}

	s.audit.Record(r.Context(), audit.Event{
		OrganizationID: principal.OrganizationID,
		Type:           audit.EventRuntimeAuthenticated,
		Outcome:        audit.OutcomeSuccess,
		ActorType:      principal.ActorType,
		ActorID:        &principal.ID,
		ActorLabel:     principal.DisplayName,
		CredentialID:   &principal.CredentialID,
		ProjectID:      &project.ID,
		EnvironmentID:  &environment.ID,
		TokenID:        &principal.CredentialID,
		IPAddress:      rc.ClientIP,
		UserAgent:      rc.UserAgent,
		Metadata: map[string]any{
			"session_ttl_seconds": int(s.runtimeSessionTTL.Seconds()),
		},
	})

	capabilities := make([]string, 0, len(scope.Capabilities))
	for _, c := range scope.Capabilities {
		capabilities = append(capabilities, string(c))
	}

	writeJSON(w, r, s.logger, http.StatusOK, runtimeAuthResponse{
		AccessToken:  token.Secret,
		ExpiresAt:    session.ExpiresAt.UTC().Format(time.RFC3339),
		Project:      project.Slug,
		Environment:  environment.Slug,
		Identity:     principal.DisplayName,
		ActorType:    string(principal.ActorType),
		Capabilities: capabilities,
	})
}

// resolveRuntimeScope determines the ceiling for the runtime session about to
// be minted.
//
// There are two cases, and keeping them apart is what stops the request body
// from becoming a privilege escalation:
//
//   - A scoped credential (a machine token, or an existing runtime session)
//     already names its project and environment. Those are used verbatim. If
//     the request also names them, they must agree; a mismatch is refused
//     rather than ignored, so a misconfigured deployment fails loudly instead
//     of quietly running against the wrong environment.
//
//   - A human's CLI login carries no scope, so the request has to say what to
//     run against. The named project and environment are resolved within the
//     caller's own organization, and the resulting session is capped at
//     USE_SECRET: a runtime session can never reveal a value, whatever its
//     holder may separately be permitted to do in the dashboard.
//
// In both cases the ceiling only narrows what the identity's grants already
// allow. Every individual key is still authorized at delivery time.
func (s *Server) resolveRuntimeScope(r *http.Request, p *domain.Principal, req runtimeAuthRequest) (*domain.CredentialScope, *APIError, error) {
	if p.Scope != nil {
		if req.Project != "" || req.Environment != "" {
			project, environment, err := s.projects.GetEnvironmentBySlug(
				r.Context(), p.OrganizationID, req.Project, req.Environment)
			if err != nil {
				return nil, translateError(err), err
			}
			if project.ID != p.Scope.ProjectID || environment.ID != p.Scope.EnvironmentID {
				return nil, ErrForbidden, errors.New("requested scope does not match the credential's scope")
			}
		}
		return p.Scope, nil, nil
	}

	// A human login. The request must name where it intends to run.
	if req.Project == "" || req.Environment == "" {
		return nil, Validation(map[string]string{
			"project":     "Name the project to run against.",
			"environment": "Name the environment to run against.",
		}), nil
	}

	project, environment, err := s.projects.GetEnvironmentBySlug(
		r.Context(), p.OrganizationID, req.Project, req.Environment)
	if err != nil {
		return nil, translateError(err), err
	}

	// The capability set is fixed rather than derived from the person's role.
	// This session exists to start a process; it is not a general credential.
	return &domain.CredentialScope{
		ProjectID:     project.ID,
		EnvironmentID: environment.ID,
		Capabilities:  []domain.Capability{domain.CapUseSecret},
	}, nil, nil
}

// runtimeSecretsRequest asks for values.
type runtimeSecretsRequest struct {
	// Keys narrows the request. Naming keys is strongly preferred: it means a
	// compromised process holds only what it actually uses, rather than
	// everything the environment contains.
	Keys []string `json:"keys"`
}

// runtimeSecretsResponse is the delivery.
//
// It contains the environment name and the values, and nothing else. No secret
// ids, no version ids, no key identifiers, no encryption detail, a runtime
// needs none of it, and every internal identifier included in a response is one
// more thing to find in a crash dump or a log aggregator.
type runtimeSecretsResponse struct {
	Environment string            `json:"environment"`
	Secrets     map[string]string `json:"secrets"`

	// Denied names requested keys the caller may not use. It does not
	// distinguish "no such key" from "not yours".
	Denied []string `json:"denied,omitempty"`

	// Unavailable names keys the caller is authorized for that have no usable
	// version. The caller has already proven access to these, so naming them
	// leaks nothing and turns a puzzling failure into a clear one.
	Unavailable []string `json:"unavailable,omitempty"`
}

// handleRuntimeSecrets delivers secret values to an authenticated runtime.
func (s *Server) handleRuntimeSecrets(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok || principal.Scope == nil {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return
	}

	var req runtimeSecretsRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, r, s.logger, ErrBadRequest, err)
			return
		}
	}

	if len(req.Keys) > maxRequestedKeys {
		writeError(w, r, s.logger, ErrBadRequest, errors.New("too many keys requested"))
		return
	}
	for _, key := range req.Keys {
		if !validSecretKey(key) {
			writeError(w, r, s.logger, Validation(map[string]string{
				"keys": "Secret keys may contain only letters, digits, and underscores.",
			}), nil)
			return
		}
	}

	environment, err := s.projects.GetEnvironment(r.Context(), principal.OrganizationID, principal.Scope.EnvironmentID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	delivery, err := s.secrets.Deliver(r.Context(), s.requestContext(r, principal), secrets.DeliverRequest{
		ProjectID:     principal.Scope.ProjectID,
		EnvironmentID: principal.Scope.EnvironmentID,
		Keys:          req.Keys,
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	// This is one of the few places plaintext deliberately crosses a boundary.
	// Expose is called explicitly, per key, immediately before serialization,
	// and the wrapped values are destroyed once the response is built.
	values := make(map[string]string, len(delivery.Values))
	for key, value := range delivery.Values {
		values[key] = value.ExposeString()
	}
	defer func() {
		for _, value := range delivery.Values {
			value.Destroy()
		}
	}()

	writeJSON(w, r, s.logger, http.StatusOK, runtimeSecretsResponse{
		Environment: environment.Slug,
		Secrets:     values,
		Denied:      delivery.Denied,
		Unavailable: delivery.Unavailable,
	})
}
