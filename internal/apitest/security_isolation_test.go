package apitest_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tobe0504/Warder/internal/apitest"
)

// The isolation guarantees. Each of these describes a way one tenant, project,
// or environment could reach another, and asserts that it cannot.

// A token minted for one project must be useless against another, even when the
// identity behind it holds access to both.
func TestTokenCannotCrossProjects(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	payments := h.NewProject(org)
	billing := h.NewProject(org)

	h.NewSecret(org, payments.DevelopmentID, "PAYMENTS_KEY", "payments-value-not-real")
	h.NewSecret(org, billing.DevelopmentID, "BILLING_KEY", "billing-value-not-real")

	// One identity, granted access to both projects.
	identityID := h.NewIdentity(org, apitest.Unique("shared"), "SERVICE")
	h.Grant(org, payments.ID, payments.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	h.Grant(org, billing.ID, billing.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)

	// But a token scoped to payments only.
	token := h.NewToken(org, payments.ID, payments.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)
	accessToken := h.RuntimeSession(token.Secret, "", "")

	// It can reach its own project.
	own := h.FetchSecrets(accessToken, []string{"PAYMENTS_KEY"})
	if _, ok := own.SecretValue("PAYMENTS_KEY"); !ok {
		t.Fatalf("the token could not reach its own project: %s", own.Raw)
	}

	// It cannot reach the other one, despite the identity's grant there.
	other := h.FetchSecrets(accessToken, []string{"BILLING_KEY"})
	if _, ok := other.SecretValue("BILLING_KEY"); ok {
		t.Fatal("a token scoped to one project reached another")
	}
	if strings.Contains(other.Raw, "billing-value-not-real") {
		t.Fatal("the response contained the other project's value")
	}
}

// The environment isolation guarantee, which is the one most often relied on:
// a development token must not be able to reach production.
func TestDevelopmentTokenCannotReachProduction(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)

	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "dev-value-not-real")
	h.NewSecret(org, project.ProductionID, "DATABASE_URL", "prod-value-not-real")

	identityID := h.NewIdentity(org, apitest.Unique("api"), "WORKLOAD")

	// The identity is granted both environments — the mistake a real
	// organization makes. The token's scope is what has to save them.
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	h.Grant(org, project.ID, project.ProductionID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)

	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)
	accessToken := h.RuntimeSession(token.Secret, "", "")

	delivery := h.FetchSecrets(accessToken, []string{"DATABASE_URL"})
	value, ok := delivery.SecretValue("DATABASE_URL")
	if !ok {
		t.Fatalf("expected the development value: %s", delivery.Raw)
	}
	if value != "dev-value-not-real" {
		t.Fatalf("a development token received a value from another environment: %q", value)
	}
	if strings.Contains(delivery.Raw, "prod-value-not-real") {
		t.Fatal("the production value appeared in a development delivery")
	}

	// Naming production explicitly must be refused, not silently ignored.
	refused := h.RuntimeCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/runtime/auth",
		Credential: token.Secret,
		Body:       map[string]string{"project": project.Slug, "environment": "production"},
	})
	if refused.Status != http.StatusForbidden {
		t.Fatalf("expected a scope mismatch to be refused, got %d: %s", refused.Status, refused.Raw)
	}
}

// Nothing crosses an organization boundary, whatever identifiers the caller
// happens to know.
func TestOrganizationsAreIsolated(t *testing.T) {
	h := apitest.New(t)

	acme := h.NewOrganization()
	acmeProject := h.NewProject(acme)
	secretID := h.NewSecret(acme, acmeProject.DevelopmentID, "ACME_SECRET", "acme-value-not-real")

	intruder := h.NewOrganization()

	// The intruder knows every identifier and presents a valid session of their
	// own. Every one of these must read as absent, not as forbidden: a 403
	// would confirm the identifier names something real.
	probes := []struct {
		name   string
		method string
		path   string
	}{
		{"read the project", http.MethodGet, "/projects/" + acmeProject.ID},
		{"list its environments", http.MethodGet, "/projects/" + acmeProject.ID + "/environments"},
		{"list its secrets", http.MethodGet, "/environments/" + acmeProject.DevelopmentID + "/secrets"},
		{"list secret versions", http.MethodGet, "/secrets/" + secretID + "/versions"},
		{"read its audit trail", http.MethodGet, "/projects/" + acmeProject.ID + "/audit"},
		{"list its tokens", http.MethodGet, "/projects/" + acmeProject.ID + "/tokens"},
		{"list its access grants", http.MethodGet, "/projects/" + acmeProject.ID + "/access"},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			resp := h.AdminCall(apitest.Request{
				Method:     probe.method,
				Path:       probe.path,
				Credential: intruder.BrowserSession,
			})
			if resp.Status != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", resp.Status, resp.Raw)
			}
			if strings.Contains(resp.Raw, "acme-value-not-real") || strings.Contains(resp.Raw, "ACME_SECRET") {
				t.Fatalf("the response leaked another organization's data: %s", resp.Raw)
			}
		})
	}

	// Nor can the intruder reveal or rotate it.
	for _, action := range []string{"reveal", "rotate"} {
		resp := h.AdminCall(apitest.Request{
			Method:     http.MethodPost,
			Path:       "/secrets/" + secretID + "/" + action,
			Credential: intruder.BrowserSession,
			Body:       map[string]string{"value": "overwritten-by-intruder"},
		})
		if resp.Status != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d: %s", action, resp.Status, resp.Raw)
		}
	}

	// And the original value is untouched.
	owner := h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/environments/" + acmeProject.DevelopmentID + "/secrets",
		Credential: acme.BrowserSession,
	})
	secrets, _ := owner.Get("secrets").([]any)
	if len(secrets) != 1 {
		t.Fatalf("the intruder altered the other organization's secrets: %s", owner.Raw)
	}
	if version, _ := secrets[0].(map[string]any)["version"].(float64); version != 1 {
		t.Fatalf("the secret was rotated by an outsider, now at version %v", version)
	}
}

// A grant scoped to one secret must not become access to its neighbours.
func TestSecretScopedGrantDoesNotLeakSiblings(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)

	allowedID := h.NewSecret(org, project.DevelopmentID, "ALLOWED_KEY", "allowed-value-not-real")
	h.NewSecret(org, project.DevelopmentID, "FORBIDDEN_KEY", "forbidden-value-not-real")

	identityID := h.NewIdentity(org, apitest.Unique("narrow"), "SERVICE")

	// A grant naming exactly one secret.
	h.MustAdmin(http.StatusCreated, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/projects/" + project.ID + "/access",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"subjectType":   "MACHINE",
			"subjectId":     identityID,
			"environmentId": project.DevelopmentID,
			"secretId":      allowedID,
			"capabilities":  []string{"USE_SECRET"},
		},
	})

	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)
	accessToken := h.RuntimeSession(token.Secret, "", "")

	// Asking for everything in the environment returns only the one secret.
	all := h.FetchSecrets(accessToken, nil)
	if _, ok := all.SecretValue("ALLOWED_KEY"); !ok {
		t.Fatalf("the granted secret was not delivered: %s", all.Raw)
	}
	if _, ok := all.SecretValue("FORBIDDEN_KEY"); ok {
		t.Fatal("a secret-scoped grant delivered a different secret")
	}
	if strings.Contains(all.Raw, "forbidden-value-not-real") {
		t.Fatal("the response contained a value the caller was not granted")
	}
}

// A token narrowed to specific keys must not deliver anything else, even where
// the identity's grant would allow it.
func TestTokenKeyScopeIsEnforced(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "db-value-not-real")
	h.NewSecret(org, project.DevelopmentID, "STRIPE_SECRET_KEY", "stripe-value-not-real")

	identityID := h.NewIdentity(org, apitest.Unique("scoped"), "AI_AGENT")
	// The environment-wide grant would allow both.
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)

	// The token names only one key.
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID,
		[]string{"USE_SECRET"}, []string{"DATABASE_URL"})
	accessToken := h.RuntimeSession(token.Secret, "", "")

	delivery := h.FetchSecrets(accessToken, nil)
	if _, ok := delivery.SecretValue("DATABASE_URL"); !ok {
		t.Fatalf("the in-scope key was not delivered: %s", delivery.Raw)
	}
	if _, ok := delivery.SecretValue("STRIPE_SECRET_KEY"); ok {
		t.Fatal("a key-scoped token delivered a key outside its scope")
	}
	if strings.Contains(delivery.Raw, "stripe-value-not-real") {
		t.Fatal("the response contained an out-of-scope value")
	}
}

// Revocation has to take effect on the next request, including for short-lived
// sessions already minted from the revoked token.
func TestRevokedTokenAndItsSessionsStopWorking(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "value-not-real")

	identityID := h.NewIdentity(org, apitest.Unique("worker"), "WORKLOAD")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	// A runtime session is minted and confirmed working.
	accessToken := h.RuntimeSession(token.Secret, "", "")
	if _, ok := h.FetchSecrets(accessToken, nil).SecretValue("DATABASE_URL"); !ok {
		t.Fatal("the secret was not delivered before revocation")
	}

	h.MustAdmin(http.StatusOK, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/tokens/" + token.ID + "/revoke",
		Credential: org.BrowserSession,
	})

	// The long-lived token no longer authenticates.
	refused := h.RuntimeCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/runtime/auth",
		Credential: token.Secret,
	})
	if refused.Status != http.StatusUnauthorized {
		t.Fatalf("a revoked token still authenticated: %d %s", refused.Status, refused.Raw)
	}

	// And the session already derived from it is dead too. Without this,
	// revoking would leave a window of up to the session lifetime in which
	// secret delivery still succeeded.
	stale := h.RuntimeCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/runtime/secrets",
		Credential: accessToken,
	})
	if stale.Status != http.StatusUnauthorized {
		t.Fatalf("a session derived from a revoked token still worked: %d %s", stale.Status, stale.Raw)
	}
	if strings.Contains(stale.Raw, "value-not-real") {
		t.Fatal("the response after revocation contained the value")
	}
}

// Temporary access must lapse on its own, with no credential rotated anywhere.
func TestTemporaryAccessExpires(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.ProductionID, "PROD_DATABASE_URL", "prod-value-not-real")

	identityID := h.NewIdentity(org, apitest.Unique("oncall"), "SERVICE")

	// Thirty minutes of production access.
	expiry := time.Now().Add(30 * time.Minute)
	grantID := h.Grant(org, project.ID, project.ProductionID, "MACHINE", identityID,
		[]string{"USE_SECRET"}, &expiry)

	token := h.NewToken(org, project.ID, project.ProductionID, identityID, []string{"USE_SECRET"}, nil)

	within := h.FetchSecrets(h.RuntimeSession(token.Secret, "", ""), nil)
	if _, ok := within.SecretValue("PROD_DATABASE_URL"); !ok {
		t.Fatalf("temporary access did not work inside its window: %s", within.Raw)
	}

	// Revoking stands in for the clock advancing; both make the grant inactive
	// through the same predicate, and neither touches the credential itself.
	h.MustAdmin(http.StatusOK, apitest.Request{
		Method:     http.MethodDelete,
		Path:       "/projects/" + project.ID + "/access/" + grantID,
		Credential: org.BrowserSession,
	})

	after := h.FetchSecrets(h.RuntimeSession(token.Secret, "", ""), nil)
	if _, ok := after.SecretValue("PROD_DATABASE_URL"); ok {
		t.Fatal("access continued after the grant ended")
	}
	if strings.Contains(after.Raw, "prod-value-not-real") {
		t.Fatal("the response after expiry contained the value")
	}

	// The secret itself is untouched: ending someone's access must not require
	// rotating the credential.
	stillThere := h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/environments/" + project.ProductionID + "/secrets",
		Credential: org.BrowserSession,
	})
	secrets, _ := stillThere.Get("secrets").([]any)
	if len(secrets) != 1 {
		t.Fatal("the secret was affected by an access change")
	}
	if version, _ := secrets[0].(map[string]any)["version"].(float64); version != 1 {
		t.Fatal("revoking access rotated the credential, which it must not do")
	}
}

// An expired secret version must not be delivered, and the caller — who is
// authorized for it — should be told why rather than left guessing.
func TestExpiredSecretIsNotDelivered(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	secretID := h.NewSecret(org, project.DevelopmentID, "TEMP_TOKEN", "temp-value-not-real")

	identityID := h.NewIdentity(org, apitest.Unique("consumer"), "SERVICE")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	if _, ok := h.FetchSecrets(h.RuntimeSession(token.Secret, "", ""), nil).SecretValue("TEMP_TOKEN"); !ok {
		t.Fatal("the secret was not delivered before expiry")
	}

	// Expire the active version directly, which is what the passage of time
	// would do.
	if _, err := h.DB.Pool.Exec(h.T.Context(),
		`UPDATE secret_versions SET expires_at = now() - interval '1 minute'
		 WHERE secret_id = $1 AND status = 'ACTIVE'`, secretID); err != nil {
		t.Fatalf("expiring the version: %v", err)
	}

	delivery := h.FetchSecrets(h.RuntimeSession(token.Secret, "", ""), []string{"TEMP_TOKEN"})
	if _, ok := delivery.SecretValue("TEMP_TOKEN"); ok {
		t.Fatal("an expired secret was delivered")
	}
	if strings.Contains(delivery.Raw, "temp-value-not-real") {
		t.Fatal("the response contained an expired value")
	}

	// The caller is authorized for this key, so distinguishing "expired" from
	// "not yours" tells them nothing they did not already know, and turns a
	// baffling outage into an actionable one.
	unavailable, _ := delivery.Get("unavailable").([]any)
	if len(unavailable) != 1 || unavailable[0] != "TEMP_TOKEN" {
		t.Fatalf("expected the key to be reported as unavailable: %s", delivery.Raw)
	}

	// The dashboard reflects it too.
	listing := h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/environments/" + project.DevelopmentID + "/secrets",
		Credential: org.BrowserSession,
	})
	secrets, _ := listing.Get("secrets").([]any)
	if status := secrets[0].(map[string]any)["status"]; status != "EXPIRED" {
		t.Fatalf("expected the dashboard to show EXPIRED, got %v", status)
	}
}
