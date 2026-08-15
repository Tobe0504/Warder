// Package authz is the single place in the system where the question "may this
// identity do this thing to this resource" is answered.
//
// No handler, service, or repository makes its own authorization judgement.
// They call Engine.Authorize and act on the decision. Centralizing this means
// the rules can be read in one file and tested exhaustively, and that adding an
// endpoint cannot accidentally introduce a weaker check than its neighbours.
package authz

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

// DenyCode is a stable, machine-readable reason for a denial, recorded in the
// audit trail. The human-readable Reason may be reworded; this may not.
type DenyCode string

const (
	DenyNone                DenyCode = ""
	DenyCrossOrganization   DenyCode = "CROSS_ORGANIZATION"
	DenyCrossProject        DenyCode = "CROSS_PROJECT"
	DenyCrossEnvironment    DenyCode = "CROSS_ENVIRONMENT"
	DenyCredentialScope     DenyCode = "CREDENTIAL_SCOPE"
	DenySecretKeyScope      DenyCode = "SECRET_KEY_SCOPE"
	DenyNoGrant             DenyCode = "NO_GRANT"
	DenyUnknownCapability   DenyCode = "UNKNOWN_CAPABILITY"
	DenyIdentityInactive    DenyCode = "IDENTITY_INACTIVE"
	DenyMalformedEvaluation DenyCode = "MALFORMED_EVALUATION"
)

// Request is a single authorization question. Unset resource fields mean the
// question is being asked at a broader level: an organization-wide request
// leaves ProjectID nil, and so on.
type Request struct {
	Principal  domain.Principal
	Capability domain.Capability

	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	SecretID       uuid.UUID

	// SecretKey is used only to apply a credential's key-level narrowing. It is
	// never used to look anything up.
	SecretKey string
}

// Decision is the outcome. It is deliberately a value rather than an error:
// callers must handle denial explicitly, and every denial carries a code that
// lands in the audit trail.
type Decision struct {
	Allowed bool
	Code    DenyCode

	// Reason is safe to show an administrator. It explains the shape of the
	// denial without revealing what else exists in the organization.
	Reason string

	// GrantID identifies the grant that permitted the request, so an
	// administrator can answer "why was this allowed" from the audit log alone.
	GrantID *uuid.UUID

	// ViaRole is set when a management capability came from the organization
	// role rather than an explicit grant.
	ViaRole bool
}

func allow(reason string) Decision {
	return Decision{Allowed: true, Reason: reason}
}

func deny(code DenyCode, reason string) Decision {
	return Decision{Allowed: false, Code: code, Reason: reason}
}

// GrantSource supplies the grants held by a subject. It is an interface so the
// engine can be tested against fixed data with no database involved.
type GrantSource interface {
	GrantsForSubject(ctx context.Context, orgID uuid.UUID, subjectType domain.SubjectType, subjectID uuid.UUID) ([]domain.AccessGrant, error)
}

// Engine evaluates authorization requests.
type Engine struct {
	grants GrantSource
	now    func() time.Time
}

// NewEngine constructs an engine. Passing a nil clock uses the wall clock.
func NewEngine(grants GrantSource, now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{grants: grants, now: now}
}

// Authorize answers a single question, denying by default.
//
// The order below matters. Tenancy and credential scope are checked before
// grants, so that a token scoped to development can never reach a production
// evaluation path at all: not even to have its grants examined. Scope is a
// structural boundary, not a permission to be weighed against others.
func (e *Engine) Authorize(ctx context.Context, req Request) (Decision, error) {
	if !domain.ValidCapability(req.Capability) {
		return deny(DenyUnknownCapability, "unrecognized capability"), nil
	}
	if req.OrganizationID == uuid.Nil || req.Principal.ID == uuid.Nil {
		return deny(DenyMalformedEvaluation, "incomplete authorization request"), nil
	}

	// Tenancy. Nothing crosses an organization boundary, ever.
	if req.Principal.OrganizationID != req.OrganizationID {
		return deny(DenyCrossOrganization, "identity belongs to a different organization"), nil
	}

	// Credential ceiling. A machine token narrows its identity and can never
	// widen it, so this runs before any grant is consulted.
	if d, ok := e.checkScope(req); !ok {
		return d, nil
	}

	// Management capabilities may come from the organization role. Capabilities
	// that can release plaintext deliberately may not: USE_SECRET and
	// READ_SECRET are only ever conferred by an explicit grant, so that being
	// an administrator is not by itself a standing claim on every credential in
	// the organization.
	if !domain.IsSecretPlane(req.Capability) && req.Principal.Type == domain.PrincipalUser {
		if slices.Contains(domain.CapabilitiesForRole(req.Principal.Role), req.Capability) {
			d := allow(fmt.Sprintf("organization role %s", req.Principal.Role))
			d.ViaRole = true
			return d, nil
		}
	}

	// Explicit grants.
	subjectType := domain.SubjectUser
	if req.Principal.Type == domain.PrincipalMachine {
		subjectType = domain.SubjectMachine
	}

	grants, err := e.grants.GrantsForSubject(ctx, req.OrganizationID, subjectType, req.Principal.ID)
	if err != nil {
		// A failure to load grants is never treated as an absence of grants.
		return Decision{}, fmt.Errorf("authz: loading grants: %w", err)
	}

	now := e.now()
	for i := range grants {
		g := &grants[i]
		if !g.Active(now) {
			continue
		}
		if !slices.Contains(g.Capabilities, req.Capability) {
			continue
		}
		if !grantCovers(g, req) {
			continue
		}
		d := allow("explicit access grant")
		id := g.ID
		d.GrantID = &id
		return d, nil
	}

	return deny(DenyNoGrant, "no active grant confers this capability on this resource"), nil
}

// checkScope applies the ceiling imposed by the presented credential.
func (e *Engine) checkScope(req Request) (Decision, bool) {
	scope := req.Principal.Scope
	if scope == nil {
		return Decision{}, true
	}

	if !scope.Allows(req.Capability) {
		return deny(DenyCredentialScope,
			"the credential used does not carry this capability"), false
	}

	// A scoped credential names one project and one environment. It therefore
	// cannot be used for organization-wide operations, and cannot address any
	// other project or environment. This is what makes a development token
	// structurally unable to reach production regardless of what its identity
	// has been granted elsewhere.
	if req.ProjectID == uuid.Nil || scope.ProjectID != req.ProjectID {
		return deny(DenyCrossProject,
			"the credential used is scoped to a different project"), false
	}
	if req.EnvironmentID == uuid.Nil || scope.EnvironmentID != req.EnvironmentID {
		return deny(DenyCrossEnvironment,
			"the credential used is scoped to a different environment"), false
	}
	if req.SecretKey != "" && !scope.AllowsKey(req.SecretKey) {
		return deny(DenySecretKeyScope,
			"the credential used is not scoped to this secret"), false
	}

	return Decision{}, true
}

// grantCovers reports whether a grant's scope covers the requested resource.
//
// A nil level on the grant is a wildcard over that level. A level named by the
// grant must match the request exactly, and a grant that names a level the
// request left unspecified does not match: a grant on one environment is not
// authority over the whole project.
func grantCovers(g *domain.AccessGrant, req Request) bool {
	if g.ProjectID != nil {
		if req.ProjectID == uuid.Nil || *g.ProjectID != req.ProjectID {
			return false
		}
	}
	if g.EnvironmentID != nil {
		if req.EnvironmentID == uuid.Nil || *g.EnvironmentID != req.EnvironmentID {
			return false
		}
	}
	if g.SecretID != nil {
		if req.SecretID == uuid.Nil || *g.SecretID != req.SecretID {
			return false
		}
	}
	return true
}

// EffectiveCapabilities reports every capability a principal currently holds
// over a resource. It exists to answer the administrator's questions: "who can
// use this secret" and "who can see it": from the same code path that enforces
// them, so the access screen cannot drift away from the real rules.
func (e *Engine) EffectiveCapabilities(ctx context.Context, req Request) ([]domain.Capability, error) {
	var held []domain.Capability
	for _, c := range domain.AllCapabilities {
		probe := req
		probe.Capability = c
		d, err := e.Authorize(ctx, probe)
		if err != nil {
			return nil, err
		}
		if d.Allowed {
			held = append(held, c)
		}
	}
	return held, nil
}
