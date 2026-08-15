package httpapi

import (
	"net/http"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/authz"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
	"github.com/google/uuid"
)

// authzTarget names the resource a handler is acting on. Unset fields mean the
// question is being asked at a broader level.
type authzTarget struct {
	ProjectID     uuid.UUID
	EnvironmentID uuid.UUID
	SecretID      uuid.UUID
	SecretKey     string
}

// allow is the single gate every admin handler passes through.
//
// Handlers never call the policy engine directly and never compare roles
// themselves. Routing every check through one function means a denial is always
// audited, always returns the same response shape, and cannot accidentally be
// written as a weaker check in one handler than in its neighbour.
//
// It returns true when the request may proceed. On denial it has already
// written the response, so the caller returns immediately.
func (s *Server) allow(w http.ResponseWriter, r *http.Request, p *domain.Principal, capability domain.Capability, target authzTarget) bool {
	decision, err := s.policy.Authorize(r.Context(), authz.Request{
		Principal:      *p,
		Capability:     capability,
		OrganizationID: p.OrganizationID,
		ProjectID:      target.ProjectID,
		EnvironmentID:  target.EnvironmentID,
		SecretID:       target.SecretID,
		SecretKey:      target.SecretKey,
	})
	if err != nil {
		// The policy engine failing is not a denial, it is an outage. Reporting
		// it as "forbidden" would send an operator hunting for a permissions
		// problem that does not exist.
		writeError(w, r, s.logger, ErrInternal, err)
		return false
	}

	if !decision.Allowed {
		s.recordDenial(r, p, capability, target, decision)
		writeError(w, r, s.logger, ErrForbidden, nil)
		return false
	}

	return true
}

func (s *Server) recordDenial(r *http.Request, p *domain.Principal, capability domain.Capability, target authzTarget, d authz.Decision) {
	ev := audit.Event{
		OrganizationID: p.OrganizationID,
		Type:           audit.EventAccessDenied,
		Outcome:        audit.OutcomeDenied,
		ActorType:      p.ActorType,
		ActorID:        &p.ID,
		ActorLabel:     p.DisplayName,
		CredentialID:   &p.CredentialID,
		SecretKey:      target.SecretKey,
		IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent:      r.UserAgent(),
		Reason:         d.Reason,
		Metadata: map[string]any{
			"capability": string(capability),
			"deny_code":  string(d.Code),
			"method":     r.Method,
			"path":       r.URL.Path,
		},
	}
	if target.ProjectID != uuid.Nil {
		ev.ProjectID = &target.ProjectID
	}
	if target.EnvironmentID != uuid.Nil {
		ev.EnvironmentID = &target.EnvironmentID
	}
	if target.SecretID != uuid.Nil {
		ev.SecretID = &target.SecretID
	}
	s.audit.Record(r.Context(), ev)
}

// secretsRepo exposes the metadata repository to handlers.
func (s *Server) secretsRepo() *store.SecretRepo { return s.secretRepo }

// authzRequestFor builds the policy question for a specific secret. It exists
// so that the display path and the enforcement path construct the request
// identically.
func authzRequestFor(p *domain.Principal, c domain.Capability, projectID, environmentID uuid.UUID, summary store.SecretSummary) authz.Request {
	return authz.Request{
		Principal:      *p,
		Capability:     c,
		OrganizationID: p.OrganizationID,
		ProjectID:      projectID,
		EnvironmentID:  environmentID,
		SecretID:       summary.ID,
		SecretKey:      summary.Key,
	}
}

// requirePrincipal fetches the authenticated principal or writes a 401.
func (s *Server) requirePrincipal(w http.ResponseWriter, r *http.Request) (*domain.Principal, bool) {
	p, ok := PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, s.logger, ErrUnauthorized, nil)
		return nil, false
	}
	return p, true
}
