package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/authz"
	"github.com/Tobe0504/Warder/internal/config"
	"github.com/Tobe0504/Warder/internal/crypto"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/identity"
	"github.com/Tobe0504/Warder/internal/ratelimit"
	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/store"
)

// Server holds the dependencies of both HTTP surfaces.
type Server struct {
	cfg    *config.Config
	logger *slog.Logger

	db         *store.DB
	accounts   *store.AccountRepo
	projects   *store.ProjectRepo
	machines   *store.MachineRepo
	grants     *store.GrantRepo
	auditLog   *store.AuditRepo
	secretRepo *store.SecretRepo

	secrets *secrets.Service
	policy  *authz.Engine
	audit   audit.Recorder
	crypto  crypto.SecretEncryptionService

	adminAuth   *identity.Chain
	runtimeAuth *identity.Chain

	limits            Limits
	runtimeSessionTTL time.Duration
	now               func() time.Time
}

// Session kinds. A session is issued for one surface and is not accepted on
// the other.
const (
	sessionKindBrowser = "BROWSER"
	sessionKindCLI     = "CLI"
)

// Limits groups the rate limiters, one per surface and sensitivity.
type Limits struct {
	Login          ratelimit.Limiter
	Invitation     ratelimit.Limiter
	Admin          ratelimit.Limiter
	Sensitive      ratelimit.Limiter
	Reveal         ratelimit.Limiter
	RuntimeAuth    ratelimit.Limiter
	RuntimeDeliver ratelimit.Limiter
}

// Deps is everything a Server needs.
type Deps struct {
	Config *config.Config
	Logger *slog.Logger

	DB         *store.DB
	Accounts   *store.AccountRepo
	Projects   *store.ProjectRepo
	Machines   *store.MachineRepo
	Grants     *store.GrantRepo
	Audit      *store.AuditRepo
	SecretRepo *store.SecretRepo

	Secrets  *secrets.Service
	Policy   *authz.Engine
	Recorder audit.Recorder
	Crypto   crypto.SecretEncryptionService

	Now func() time.Time
}

// New constructs the server and its authentication chains.
func New(d Deps) *Server {
	now := d.Now
	if now == nil {
		now = time.Now
	}

	return &Server{
		cfg:        d.Config,
		logger:     d.Logger,
		db:         d.DB,
		accounts:   d.Accounts,
		projects:   d.Projects,
		machines:   d.Machines,
		grants:     d.Grants,
		auditLog:   d.Audit,
		secretRepo: d.SecretRepo,
		secrets:    d.Secrets,
		policy:     d.Policy,
		audit:      d.Recorder,
		crypto:     d.Crypto,

		// The two surfaces accept different credentials, and the separation is
		// structural rather than a check inside a handler.
		//
		// The admin chain accepts only browser sessions. A machine token
		// presented to the dashboard API is not recognized at all, so a stolen
		// workload credential cannot be used to browse or administer an
		// organization — and a CLI login sitting in a file on a laptop cannot
		// be used to change access policy.
		//
		// The runtime chain accepts machine tokens, runtime sessions, and CLI
		// logins. That last one is what lets a developer run `ward run` as
		// themselves, bounded by their own grants. Browser sessions are refused
		// here: the cookie the dashboard holds is the credential most exposed
		// to a cross-site scripting flaw, and it must not be a path to secret
		// delivery.
		adminAuth: identity.NewChain(
			identity.NewUserSessionProvider(d.Accounts, now, sessionKindBrowser),
		),
		runtimeAuth: identity.NewChain(
			identity.NewMachineTokenProvider(d.Machines, now),
			identity.NewRuntimeSessionProvider(d.Machines, now),
			identity.NewUserSessionProvider(d.Accounts, now, sessionKindCLI),
		),

		limits: Limits{
			Login:          ratelimit.New(ratelimit.Login),
			Invitation:     ratelimit.New(ratelimit.Invitation),
			Admin:          ratelimit.New(ratelimit.Admin),
			Sensitive:      ratelimit.New(ratelimit.Sensitive),
			Reveal:         ratelimit.New(ratelimit.Reveal),
			RuntimeAuth:    ratelimit.New(ratelimit.RuntimeAuth),
			RuntimeDeliver: ratelimit.New(ratelimit.RuntimeDeliver),
		},
		runtimeSessionTTL: d.Config.RuntimeSessionTTL,
		now:               now,
	}
}

// AdminHandler builds the human-facing API, reachable only by the BFF.
//
// Every route here is wrapped in the service-token check, so possession of a
// user session is never sufficient on its own. That is the mechanism behind
// "the browser never calls the core API".
func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)

	// Authentication. These run before a principal exists, so they are keyed by
	// client address and sit behind the login limiter.
	mux.Handle("POST /auth/login", s.public(s.limits.Login, http.HandlerFunc(s.handleLogin)))
	mux.Handle("POST /auth/accept-invitation", s.public(s.limits.Invitation, http.HandlerFunc(s.handleAcceptInvitation)))
	mux.Handle("POST /auth/logout", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleLogout)))
	mux.Handle("GET /auth/session", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleSession)))

	// Bootstrap: creating the first organization, owner, and their project.
	mux.Handle("POST /organizations", s.public(s.limits.Sensitive, http.HandlerFunc(s.handleCreateOrganization)))

	mux.Handle("GET /projects", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListProjects)))
	mux.Handle("POST /projects", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleCreateProject)))
	mux.Handle("GET /projects/{projectID}", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleGetProject)))

	mux.Handle("GET /projects/{projectID}/environments", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListEnvironments)))
	mux.Handle("POST /projects/{projectID}/environments", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleCreateEnvironment)))

	mux.Handle("GET /environments/{environmentID}/secrets", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListSecrets)))
	mux.Handle("POST /environments/{environmentID}/secrets", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleCreateSecret)))
	// One transaction for a whole .env import, so a paste either lands or does
	// not. Looping the route above from a browser would do neither reliably.
	mux.Handle("POST /environments/{environmentID}/secrets/batch", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleCreateSecrets)))

	mux.Handle("GET /secrets/{secretID}/versions", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListVersions)))
	mux.Handle("POST /secrets/{secretID}/rotate", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleRotateSecret)))
	mux.Handle("POST /secrets/{secretID}/revoke", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleRevokeVersion)))
	mux.Handle("POST /secrets/{secretID}/rollback", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleRollbackVersion)))
	mux.Handle("POST /secrets/{secretID}/expiry", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleSetExpiry)))

	// Reveal has its own, much tighter limiter. It is the only admin route that
	// can return plaintext.
	mux.Handle("POST /secrets/{secretID}/reveal", s.authenticated(s.limits.Reveal, http.HandlerFunc(s.handleRevealSecret)))

	mux.Handle("GET /projects/{projectID}/access", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListAccess)))
	mux.Handle("POST /projects/{projectID}/access", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleGrantAccess)))
	mux.Handle("DELETE /projects/{projectID}/access/{grantID}", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleRevokeAccess)))

	mux.Handle("GET /identities", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListIdentities)))
	mux.Handle("POST /identities", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleCreateIdentity)))
	mux.Handle("POST /identities/{identityID}/disable", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleDisableIdentity)))

	mux.Handle("GET /projects/{projectID}/tokens", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListTokens)))
	mux.Handle("POST /projects/{projectID}/tokens", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleCreateToken)))
	mux.Handle("POST /tokens/{tokenID}/revoke", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleRevokeToken)))

	mux.Handle("GET /projects/{projectID}/audit", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleProjectAudit)))
	mux.Handle("GET /audit", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleOrganizationAudit)))

	mux.Handle("GET /members", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListMembers)))
	mux.Handle("DELETE /members/{membershipID}", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleRemoveMember)))

	// Membership is offered, not assigned. There is deliberately no route that
	// creates an account with a password chosen by somebody else.
	mux.Handle("GET /members/invitations", s.authenticated(s.limits.Admin, http.HandlerFunc(s.handleListInvitations)))
	mux.Handle("POST /members/invitations", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleCreateInvitation)))
	mux.Handle("DELETE /members/invitations/{invitationID}", s.authenticated(s.limits.Sensitive, http.HandlerFunc(s.handleRevokeInvitation)))

	return chain(mux,
		withRequestID,
		withRecovery(s.logger),
		withSecurityHeaders,
		withRequestLogging(s.logger, s.cfg.TrustProxyHeaders),
		withMaxBody(1<<20),
		withServiceToken(s.cfg.ServiceToken, s.logger),
	)
}

// RuntimeHandler builds the machine-to-machine API.
//
// It is returned separately so it can be served on its own listener and
// address. The BFF has no route that reaches it, and in a production deployment
// it should sit on a network the dashboard's backend cannot address at all.
//
// Note what is absent: the service-token middleware. A runtime authenticates as
// itself, and requiring a shared service credential as well would mean shipping
// that credential to every workload — turning one stolen container into a
// foothold on the human-facing API.
func (s *Server) RuntimeHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)

	mux.Handle("POST /runtime/auth", chain(http.HandlerFunc(s.handleRuntimeAuth),
		withRateLimit(s.limits.RuntimeAuth, byClientIP(s.cfg.TrustProxyHeaders), s.logger),
		withAuthentication(s.runtimeAuth, s.cfg.TrustProxyHeaders, s.logger),
	))

	mux.Handle("POST /runtime/secrets", chain(http.HandlerFunc(s.handleRuntimeSecrets),
		withAuthentication(s.runtimeAuth, s.cfg.TrustProxyHeaders, s.logger),
		// Keyed by the authenticated identity rather than the address, so that
		// many workloads behind one NAT gateway do not throttle each other, and
		// a single misbehaving identity is contained.
		withRateLimit(s.limits.RuntimeDeliver, byPrincipal, s.logger),
	))

	// The CLI's own routes.
	//
	// They live here rather than on the admin surface because the CLI is a
	// machine client: it authenticates with a credential it holds, and it
	// cannot be given the BFF's service token without shipping that credential
	// to every developer's laptop — which would end the guarantee that only the
	// BFF can reach the human-facing API.
	//
	// Everything here is read-only metadata plus login. There is no route on
	// this surface that changes access policy, and none that returns a
	// plaintext value except the deliberate delivery endpoint above.
	mux.Handle("POST /cli/login", chain(http.HandlerFunc(s.handleLogin),
		withRateLimit(s.limits.Login, byClientIP(s.cfg.TrustProxyHeaders), s.logger),
	))
	mux.Handle("POST /cli/logout", chain(http.HandlerFunc(s.handleLogout),
		withAuthentication(s.runtimeAuth, s.cfg.TrustProxyHeaders, s.logger),
		withRateLimit(s.limits.Admin, byPrincipal, s.logger),
	))
	mux.Handle("GET /cli/projects", chain(http.HandlerFunc(s.handleListProjects),
		withAuthentication(s.runtimeAuth, s.cfg.TrustProxyHeaders, s.logger),
		withRateLimit(s.limits.Admin, byPrincipal, s.logger),
	))
	mux.Handle("GET /cli/projects/{projectID}/environments", chain(http.HandlerFunc(s.handleListEnvironments),
		withAuthentication(s.runtimeAuth, s.cfg.TrustProxyHeaders, s.logger),
		withRateLimit(s.limits.Admin, byPrincipal, s.logger),
	))
	mux.Handle("GET /cli/environments/{environmentID}/secrets", chain(http.HandlerFunc(s.handleListSecrets),
		withAuthentication(s.runtimeAuth, s.cfg.TrustProxyHeaders, s.logger),
		withRateLimit(s.limits.Admin, byPrincipal, s.logger),
	))

	return chain(mux,
		withRequestID,
		withRecovery(s.logger),
		withSecurityHeaders,
		withRequestLogging(s.logger, s.cfg.TrustProxyHeaders),
		withMaxBody(64<<10),
	)
}

// public wraps a route that runs before authentication.
func (s *Server) public(limiter ratelimit.Limiter, h http.Handler) http.Handler {
	return chain(h, withRateLimit(limiter, byClientIP(s.cfg.TrustProxyHeaders), s.logger))
}

// authenticated wraps a route that requires a principal.
func (s *Server) authenticated(limiter ratelimit.Limiter, h http.Handler) http.Handler {
	return chain(h,
		withAuthentication(s.adminAuth, s.cfg.TrustProxyHeaders, s.logger),
		withRateLimit(limiter, byPrincipal, s.logger),
	)
}

func byClientIP(trustProxy bool) func(*http.Request) string {
	return func(r *http.Request) string { return "ip:" + ClientIP(r, trustProxy) }
}

func byPrincipal(r *http.Request) string {
	if p, ok := PrincipalFrom(r.Context()); ok {
		return "principal:" + p.ID.String()
	}
	return "anonymous"
}

// requestContext assembles the audit context for a request.
func (s *Server) requestContext(r *http.Request, p *domain.Principal) secrets.RequestContext {
	return secrets.RequestContext{
		Principal: *p,
		ClientIP:  ClientIP(r, s.cfg.TrustProxyHeaders),
		UserAgent: r.UserAgent(),
	}
}

// handleHealth reports liveness.
//
// It reports only that the process is up. Version numbers, key provider
// descriptions, and database status are all useful to an operator and equally
// useful to someone deciding whether this deployment is worth attacking, so
// they belong on an internal endpoint rather than this one.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, s.logger, http.StatusOK, map[string]string{"status": "ok"})
}
