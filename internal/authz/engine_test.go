package authz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

// fixedGrants is an in-memory GrantSource.
type fixedGrants struct {
	grants []domain.AccessGrant
	err    error
}

func (f *fixedGrants) GrantsForSubject(_ context.Context, orgID uuid.UUID, st domain.SubjectType, sid uuid.UUID) ([]domain.AccessGrant, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.AccessGrant
	for _, g := range f.grants {
		if g.OrganizationID == orgID && g.SubjectType == st && g.SubjectID == sid {
			out = append(out, g)
		}
	}
	return out, nil
}

// world is a small fixture: one organization, one project, two environments.
type world struct {
	org         uuid.UUID
	project     uuid.UUID
	otherProj   uuid.UUID
	development uuid.UUID
	production  uuid.UUID
	databaseURL uuid.UUID
	stripeKey   uuid.UUID
}

func newWorld() world {
	return world{
		org:         uuid.New(),
		project:     uuid.New(),
		otherProj:   uuid.New(),
		development: uuid.New(),
		production:  uuid.New(),
		databaseURL: uuid.New(),
		stripeKey:   uuid.New(),
	}
}

func (w world) req(p domain.Principal, c domain.Capability) Request {
	return Request{
		Principal:      p,
		Capability:     c,
		OrganizationID: w.org,
		ProjectID:      w.project,
		EnvironmentID:  w.development,
		SecretID:       w.databaseURL,
	}
}

func (w world) user(role domain.Role) domain.Principal {
	return domain.Principal{
		Type:           domain.PrincipalUser,
		ID:             uuid.New(),
		OrganizationID: w.org,
		ActorType:      domain.ActorHuman,
		Role:           role,
		DisplayName:    "test user",
	}
}

func (w world) machine(actor domain.ActorType, scope *domain.CredentialScope) domain.Principal {
	return domain.Principal{
		Type:           domain.PrincipalMachine,
		ID:             uuid.New(),
		OrganizationID: w.org,
		ActorType:      actor,
		Scope:          scope,
		DisplayName:    "test machine",
	}
}

func grant(w world, p domain.Principal, caps ...domain.Capability) domain.AccessGrant {
	st := domain.SubjectUser
	if p.Type == domain.PrincipalMachine {
		st = domain.SubjectMachine
	}
	proj, env := w.project, w.development
	return domain.AccessGrant{
		ID:             uuid.New(),
		OrganizationID: w.org,
		SubjectType:    st,
		SubjectID:      p.ID,
		ProjectID:      &proj,
		EnvironmentID:  &env,
		Capabilities:   caps,
	}
}

func engineWith(grants ...domain.AccessGrant) *Engine {
	return NewEngine(&fixedGrants{grants: grants}, nil)
}

func mustAuthorize(t *testing.T, e *Engine, req Request) Decision {
	t.Helper()
	d, err := e.Authorize(context.Background(), req)
	if err != nil {
		t.Fatalf("authorize returned an error: %v", err)
	}
	return d
}

// ---------------------------------------------------------------------------
// The central product claim: using a secret and seeing it are separate.
// ---------------------------------------------------------------------------

func TestUseSecretDoesNotImplyReadSecret(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)
	e := engineWith(grant(w, dev, domain.CapUseSecret))

	if d := mustAuthorize(t, e, w.req(dev, domain.CapUseSecret)); !d.Allowed {
		t.Fatalf("developer should be able to use the secret: %s", d.Reason)
	}
	if d := mustAuthorize(t, e, w.req(dev, domain.CapReadSecret)); d.Allowed {
		t.Fatal("USE_SECRET conferred READ_SECRET")
	}
}

func TestReadSecretDoesNotImplyUseSecret(t *testing.T) {
	w := newWorld()
	auditor := w.user(domain.RoleViewer)
	e := engineWith(grant(w, auditor, domain.CapReadSecret))

	if d := mustAuthorize(t, e, w.req(auditor, domain.CapReadSecret)); !d.Allowed {
		t.Fatalf("expected READ_SECRET to be allowed: %s", d.Reason)
	}
	if d := mustAuthorize(t, e, w.req(auditor, domain.CapUseSecret)); d.Allowed {
		t.Fatal("READ_SECRET conferred USE_SECRET")
	}
}

// No role anywhere confers plaintext access. An owner who has granted
// themselves nothing can administer the platform and still not read a value.
func TestNoRoleConfersPlaintextAccess(t *testing.T) {
	w := newWorld()

	for _, role := range []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleDeveloper, domain.RoleViewer} {
		t.Run(string(role), func(t *testing.T) {
			p := w.user(role)
			e := engineWith() // no grants at all

			for _, c := range []domain.Capability{domain.CapUseSecret, domain.CapReadSecret} {
				if d := mustAuthorize(t, e, w.req(p, c)); d.Allowed {
					t.Fatalf("role %s conferred %s without a grant", role, c)
				}
			}
		})
	}
}

// Management capabilities do come from roles; that is the point of the split.
func TestRolesConferManagementCapabilities(t *testing.T) {
	w := newWorld()
	owner := w.user(domain.RoleOwner)
	viewer := w.user(domain.RoleViewer)
	e := engineWith()

	d := mustAuthorize(t, e, w.req(owner, domain.CapManageAccess))
	if !d.Allowed || !d.ViaRole {
		t.Fatalf("owner should hold MANAGE_ACCESS through their role: %+v", d)
	}
	if d := mustAuthorize(t, e, w.req(viewer, domain.CapManageAccess)); d.Allowed {
		t.Fatal("viewer holds MANAGE_ACCESS")
	}
	if d := mustAuthorize(t, e, w.req(viewer, domain.CapReadMetadata)); !d.Allowed {
		t.Fatal("viewer should be able to see that secrets exist")
	}
}

// ---------------------------------------------------------------------------
// Isolation
// ---------------------------------------------------------------------------

func TestOrganizationIsolation(t *testing.T) {
	w := newWorld()
	outsider := w.user(domain.RoleOwner)
	outsider.OrganizationID = uuid.New() // belongs elsewhere

	e := engineWith(grant(w, outsider, domain.CapUseSecret, domain.CapReadSecret))

	d := mustAuthorize(t, e, w.req(outsider, domain.CapUseSecret))
	if d.Allowed {
		t.Fatal("an identity from another organization was authorized")
	}
	if d.Code != DenyCrossOrganization {
		t.Fatalf("expected %s, got %s", DenyCrossOrganization, d.Code)
	}
}

func TestGrantOnOneEnvironmentDoesNotReachAnother(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)
	e := engineWith(grant(w, dev, domain.CapUseSecret)) // development only

	req := w.req(dev, domain.CapUseSecret)
	req.EnvironmentID = w.production
	req.SecretID = uuid.New()

	if d := mustAuthorize(t, e, req); d.Allowed {
		t.Fatal("a development grant reached production")
	}
}

func TestGrantOnOneProjectDoesNotReachAnother(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)
	e := engineWith(grant(w, dev, domain.CapUseSecret))

	req := w.req(dev, domain.CapUseSecret)
	req.ProjectID = w.otherProj

	if d := mustAuthorize(t, e, req); d.Allowed {
		t.Fatal("a grant on one project reached another")
	}
}

// A grant naming a single secret must not become authority over its neighbours.
func TestSecretScopedGrantDoesNotCoverSiblings(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)

	g := grant(w, dev, domain.CapUseSecret)
	g.SecretID = &w.databaseURL
	e := engineWith(g)

	if d := mustAuthorize(t, e, w.req(dev, domain.CapUseSecret)); !d.Allowed {
		t.Fatalf("grant should cover its own secret: %s", d.Reason)
	}

	sibling := w.req(dev, domain.CapUseSecret)
	sibling.SecretID = w.stripeKey
	if d := mustAuthorize(t, e, sibling); d.Allowed {
		t.Fatal("a secret-scoped grant covered a different secret")
	}
}

// A grant on one environment must not satisfy a question asked at the whole
// project level, which would turn narrow access into broad access.
func TestNarrowGrantDoesNotSatisfyBroaderQuestion(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)
	e := engineWith(grant(w, dev, domain.CapUseSecret))

	broad := w.req(dev, domain.CapUseSecret)
	broad.EnvironmentID = uuid.Nil
	broad.SecretID = uuid.Nil

	if d := mustAuthorize(t, e, broad); d.Allowed {
		t.Fatal("an environment-scoped grant answered a project-wide question")
	}
}

// ---------------------------------------------------------------------------
// Credential scope: a token narrows an identity and can never widen it.
// ---------------------------------------------------------------------------

func TestTokenScopeCannotWidenIdentity(t *testing.T) {
	w := newWorld()

	// The token claims READ_SECRET, but the identity was never granted it.
	scope := &domain.CredentialScope{
		ProjectID:     w.project,
		EnvironmentID: w.development,
		Capabilities:  []domain.Capability{domain.CapUseSecret, domain.CapReadSecret},
	}
	agent := w.machine(domain.ActorAIAgent, scope)
	e := engineWith(grant(w, agent, domain.CapUseSecret))

	if d := mustAuthorize(t, e, w.req(agent, domain.CapUseSecret)); !d.Allowed {
		t.Fatalf("agent should be able to use the secret: %s", d.Reason)
	}
	d := mustAuthorize(t, e, w.req(agent, domain.CapReadSecret))
	if d.Allowed {
		t.Fatal("a token widened its identity's authority")
	}
	if d.Code != DenyNoGrant {
		t.Fatalf("expected the identity's grants to be the binding constraint, got %s", d.Code)
	}
}

func TestTokenScopeNarrowsIdentity(t *testing.T) {
	w := newWorld()

	// The identity is broadly granted, but the token only carries USE_SECRET.
	scope := &domain.CredentialScope{
		ProjectID:     w.project,
		EnvironmentID: w.development,
		Capabilities:  []domain.Capability{domain.CapUseSecret},
	}
	workload := w.machine(domain.ActorWorkload, scope)
	e := engineWith(grant(w, workload, domain.CapUseSecret, domain.CapReadSecret, domain.CapRotateSecret))

	if d := mustAuthorize(t, e, w.req(workload, domain.CapUseSecret)); !d.Allowed {
		t.Fatalf("expected USE_SECRET: %s", d.Reason)
	}

	d := mustAuthorize(t, e, w.req(workload, domain.CapReadSecret))
	if d.Allowed {
		t.Fatal("a narrow token exercised a capability it does not carry")
	}
	if d.Code != DenyCredentialScope {
		t.Fatalf("expected %s, got %s", DenyCredentialScope, d.Code)
	}
}

// The structural guarantee behind "a development token must not work in
// production": scope is checked before grants, so a production grant on the
// identity is never even consulted.
func TestDevelopmentTokenCannotReachProduction(t *testing.T) {
	w := newWorld()

	scope := &domain.CredentialScope{
		ProjectID:     w.project,
		EnvironmentID: w.development,
		Capabilities:  []domain.Capability{domain.CapUseSecret},
	}
	ci := w.machine(domain.ActorCI, scope)

	// The identity is separately granted production access.
	prodGrant := grant(w, ci, domain.CapUseSecret)
	prodGrant.EnvironmentID = &w.production
	e := engineWith(prodGrant)

	req := w.req(ci, domain.CapUseSecret)
	req.EnvironmentID = w.production

	d := mustAuthorize(t, e, req)
	if d.Allowed {
		t.Fatal("a development-scoped token reached production")
	}
	if d.Code != DenyCrossEnvironment {
		t.Fatalf("scope must be the binding constraint, got %s", d.Code)
	}
}

func TestTokenKeyScopeNarrowsToSpecificSecrets(t *testing.T) {
	w := newWorld()

	scope := &domain.CredentialScope{
		ProjectID:     w.project,
		EnvironmentID: w.development,
		Capabilities:  []domain.Capability{domain.CapUseSecret},
		SecretKeys:    []string{"DATABASE_URL"},
	}
	workload := w.machine(domain.ActorWorkload, scope)
	e := engineWith(grant(w, workload, domain.CapUseSecret))

	permitted := w.req(workload, domain.CapUseSecret)
	permitted.SecretKey = "DATABASE_URL"
	if d := mustAuthorize(t, e, permitted); !d.Allowed {
		t.Fatalf("expected the in-scope key to be allowed: %s", d.Reason)
	}

	refused := w.req(workload, domain.CapUseSecret)
	refused.SecretKey = "STRIPE_SECRET_KEY"
	d := mustAuthorize(t, e, refused)
	if d.Allowed {
		t.Fatal("a key-scoped token reached a key outside its scope")
	}
	if d.Code != DenySecretKeyScope {
		t.Fatalf("expected %s, got %s", DenySecretKeyScope, d.Code)
	}
}

// A scoped credential is confined to its project and environment, so it cannot
// be used to perform organization-wide operations.
func TestScopedCredentialCannotActOrganizationWide(t *testing.T) {
	w := newWorld()
	scope := &domain.CredentialScope{
		ProjectID:     w.project,
		EnvironmentID: w.development,
		Capabilities:  []domain.Capability{domain.CapUseSecret, domain.CapManageOrganization},
	}
	svc := w.machine(domain.ActorService, scope)

	g := domain.AccessGrant{
		ID: uuid.New(), OrganizationID: w.org,
		SubjectType: domain.SubjectMachine, SubjectID: svc.ID,
		Capabilities: []domain.Capability{domain.CapManageOrganization},
	}
	e := engineWith(g)

	req := w.req(svc, domain.CapManageOrganization)
	req.ProjectID = uuid.Nil
	req.EnvironmentID = uuid.Nil
	req.SecretID = uuid.Nil

	if d := mustAuthorize(t, e, req); d.Allowed {
		t.Fatal("a project-scoped credential performed an organization-wide operation")
	}
}

// ---------------------------------------------------------------------------
// The AI agent case, stated as its own test because it is the product's
// motivating example.
// ---------------------------------------------------------------------------

func TestAIAgentCanUseDevelopmentButNotReadAndNotReachProduction(t *testing.T) {
	w := newWorld()

	scope := &domain.CredentialScope{
		ProjectID:     w.project,
		EnvironmentID: w.development,
		Capabilities:  []domain.Capability{domain.CapUseSecret},
	}
	agent := w.machine(domain.ActorAIAgent, scope)
	e := engineWith(grant(w, agent, domain.CapUseSecret))

	// It can run the tests.
	if d := mustAuthorize(t, e, w.req(agent, domain.CapUseSecret)); !d.Allowed {
		t.Fatalf("agent cannot use development credentials: %s", d.Reason)
	}

	// It cannot print them.
	if d := mustAuthorize(t, e, w.req(agent, domain.CapReadSecret)); d.Allowed {
		t.Fatal("agent can reveal a credential")
	}

	// It cannot rotate or revoke anything.
	for _, c := range []domain.Capability{domain.CapRotateSecret, domain.CapRevokeSecret, domain.CapManageAccess, domain.CapCreateSecret} {
		if d := mustAuthorize(t, e, w.req(agent, c)); d.Allowed {
			t.Fatalf("agent holds %s", c)
		}
	}

	// It cannot reach staging or production.
	for _, env := range []uuid.UUID{w.production, uuid.New()} {
		req := w.req(agent, domain.CapUseSecret)
		req.EnvironmentID = env
		if d := mustAuthorize(t, e, req); d.Allowed {
			t.Fatal("agent reached an environment outside its scope")
		}
	}
}

// An agent must not pick up authority just because a human created it.
func TestAgentDoesNotInheritCreatorAuthority(t *testing.T) {
	w := newWorld()

	owner := w.user(domain.RoleOwner)
	ownerGrant := grant(w, owner, domain.CapUseSecret, domain.CapReadSecret)

	scope := &domain.CredentialScope{
		ProjectID:     w.project,
		EnvironmentID: w.development,
		Capabilities:  []domain.Capability{domain.CapUseSecret},
	}
	agent := w.machine(domain.ActorAIAgent, scope)

	e := engineWith(ownerGrant) // only the owner is granted anything

	if d := mustAuthorize(t, e, w.req(agent, domain.CapUseSecret)); d.Allowed {
		t.Fatal("an agent inherited its creator's access")
	}
}

// ---------------------------------------------------------------------------
// Time-bounded access
// ---------------------------------------------------------------------------

func TestExpiredGrantConfersNothing(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)

	g := grant(w, dev, domain.CapUseSecret)
	expiry := time.Now().Add(30 * time.Minute)
	g.ExpiresAt = &expiry

	source := &fixedGrants{grants: []domain.AccessGrant{g}}

	// Inside the window.
	before := NewEngine(source, func() time.Time { return expiry.Add(-time.Minute) })
	if d := mustAuthorize(t, before, w.req(dev, domain.CapUseSecret)); !d.Allowed {
		t.Fatalf("temporary access should work inside its window: %s", d.Reason)
	}

	// After it, with no credential anywhere having been rotated.
	after := NewEngine(source, func() time.Time { return expiry.Add(time.Second) })
	d := mustAuthorize(t, after, w.req(dev, domain.CapUseSecret))
	if d.Allowed {
		t.Fatal("expired temporary access still worked")
	}
	if d.Code != DenyNoGrant {
		t.Fatalf("expected %s, got %s", DenyNoGrant, d.Code)
	}
}

func TestRevokedGrantConfersNothing(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)

	g := grant(w, dev, domain.CapUseSecret)
	revoked := time.Now().Add(-time.Minute)
	g.RevokedAt = &revoked

	e := engineWith(g)
	if d := mustAuthorize(t, e, w.req(dev, domain.CapUseSecret)); d.Allowed {
		t.Fatal("a revoked grant still conferred access")
	}
}

// ---------------------------------------------------------------------------
// Failure behaviour
// ---------------------------------------------------------------------------

// A grant store that is unavailable must not read as "this identity has no
// grants", which would be a silent, total denial — or worse, if the logic were
// inverted, a silent allow. It must surface as an error the caller cannot
// mistake for a decision.
func TestGrantLoadFailureIsNotTreatedAsAbsenceOfGrants(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)
	boom := errors.New("database unavailable")

	e := NewEngine(&fixedGrants{err: boom}, nil)
	d, err := e.Authorize(context.Background(), w.req(dev, domain.CapUseSecret))

	if err == nil {
		t.Fatal("a grant store failure produced a decision instead of an error")
	}
	if d.Allowed {
		t.Fatal("a grant store failure produced an allow")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected the underlying cause to be preserved, got %v", err)
	}
}

func TestUnknownCapabilityIsDenied(t *testing.T) {
	w := newWorld()
	owner := w.user(domain.RoleOwner)
	e := engineWith()

	req := w.req(owner, domain.Capability("SUDO"))
	d := mustAuthorize(t, e, req)
	if d.Allowed {
		t.Fatal("an unrecognized capability was allowed")
	}
	if d.Code != DenyUnknownCapability {
		t.Fatalf("expected %s, got %s", DenyUnknownCapability, d.Code)
	}
}

func TestDefaultIsDenial(t *testing.T) {
	w := newWorld()
	stranger := w.user(domain.RoleDeveloper)
	e := engineWith()

	for _, c := range domain.AllCapabilities {
		d := mustAuthorize(t, e, w.req(stranger, c))
		if d.Allowed && !d.ViaRole {
			t.Fatalf("%s was allowed with no grant and no role backing it", c)
		}
	}
}

// EffectiveCapabilities drives the access screen, so it must agree exactly with
// what enforcement does — a screen that overstates access is a security bug.
func TestEffectiveCapabilitiesMatchesEnforcement(t *testing.T) {
	w := newWorld()
	dev := w.user(domain.RoleDeveloper)
	e := engineWith(grant(w, dev, domain.CapUseSecret))

	held, err := e.EffectiveCapabilities(context.Background(), w.req(dev, ""))
	if err != nil {
		t.Fatalf("effective capabilities: %v", err)
	}

	for _, c := range domain.AllCapabilities {
		d := mustAuthorize(t, e, w.req(dev, c))
		listed := false
		for _, h := range held {
			if h == c {
				listed = true
			}
		}
		if listed != d.Allowed {
			t.Fatalf("%s: access screen says %v, enforcement says %v", c, listed, d.Allowed)
		}
	}
}
