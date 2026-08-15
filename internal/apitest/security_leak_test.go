package apitest_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Tobe0504/Warder/internal/apitest"
)

// These tests all ask one question in different places: can a secret value get
// out somewhere it should not? Each targets a specific channel, the logs, an
// error body, a browser response, the audit trail.

// The canonical requirement: the logs should name DATABASE_URL and never say
// what it is.
func TestLogsNameSecretsButNeverTheirValues(t *testing.T) {
	h := apitest.New(t)

	const value = "postgres://user:log-canary-password@db.internal:5432/app"

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", value)

	identityID := h.NewIdentity(org, apitest.Unique("api"), "WORKLOAD")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	accessToken := h.RuntimeSession(token.Secret, "", "")
	if _, ok := h.FetchSecrets(accessToken, []string{"DATABASE_URL"}).SecretValue("DATABASE_URL"); !ok {
		t.Fatal("the secret was not delivered")
	}

	// Read the version history and audit too, exercising more log paths.
	h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/projects/" + project.ID + "/audit",
		Credential: org.BrowserSession,
	})

	logs := h.Logs.String()

	for _, forbidden := range []string{value, "log-canary-password"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("the logs contain secret material: %q", forbidden)
		}
	}

	// Credentials must not be in the logs either.
	for _, credential := range []string{token.Secret, accessToken, org.BrowserSession, org.CLISession} {
		if strings.Contains(logs, credential) {
			t.Fatal("the logs contain a credential")
		}
	}

	// But the logs must still be useful: the request path and the actor should
	// be there. Redaction that removes everything is not a win.
	if !strings.Contains(logs, "/runtime/secrets") {
		t.Fatal("the logs should record which endpoint was called")
	}
}

// Forcing a decryption failure must produce a generic error, not a description
// of what went wrong with the cryptography.
func TestDecryptionFailureLeaksNothing(t *testing.T) {
	h := apitest.New(t)

	const value = "sk_live_decryption_failure_canary"

	org := h.NewOrganization()
	project := h.NewProject(org)
	secretID := h.NewSecret(org, project.DevelopmentID, "STRIPE_SECRET_KEY", value)

	identityID := h.NewIdentity(org, apitest.Unique("api"), "SERVICE")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	// Corrupt the stored ciphertext, simulating tampering or bit rot.
	if _, err := h.DB.Pool.Exec(h.T.Context(), `
		UPDATE secret_material.secret_version_material m
		SET ciphertext = decode('00', 'hex') || m.ciphertext
		FROM secret_versions v
		WHERE v.id = m.secret_version_id AND v.secret_id = $1`, secretID); err != nil {
		t.Fatalf("corrupting the ciphertext: %v", err)
	}

	delivery := h.FetchSecrets(h.RuntimeSession(token.Secret, "", ""), []string{"STRIPE_SECRET_KEY"})

	if _, ok := delivery.SecretValue("STRIPE_SECRET_KEY"); ok {
		t.Fatal("a corrupted secret was delivered")
	}
	if strings.Contains(delivery.Raw, value) {
		t.Fatal("the response contained the plaintext")
	}

	// The caller learns the secret is unavailable, and nothing about why. No
	// mention of authentication tags, key versions, or nonces, each of which
	// would tell an attacker which part of a forgery attempt was wrong.
	for _, forbidden := range []string{"authentication", "cipher", "nonce", "key_id", "local:", "wrapped", "GCM", "aead"} {
		if strings.Contains(strings.ToLower(delivery.Raw), strings.ToLower(forbidden)) {
			t.Fatalf("the response described the cryptographic failure (%q): %s", forbidden, delivery.Raw)
		}
	}

	unavailable, _ := delivery.Get("unavailable").([]any)
	if len(unavailable) != 1 {
		t.Fatalf("expected the key to be reported unavailable: %s", delivery.Raw)
	}

	// The operator, unlike the caller, does get the detail, in a log line that
	// still carries no plaintext.
	logs := h.Logs.String()
	if !strings.Contains(logs, "could not be decrypted") {
		t.Fatal("the decryption failure was not logged for operators")
	}
	if strings.Contains(logs, value) {
		t.Fatal("the failure log contained the plaintext")
	}
}

// The browser's own API must not carry a value anywhere, in any response, for
// any caller who has not explicitly asked to reveal one.
func TestBrowserAPINeverCarriesPlaintextIncidentally(t *testing.T) {
	h := apitest.New(t)

	const value = "browser-canary-value-not-real"

	org := h.NewOrganization()
	project := h.NewProject(org)
	secretID := h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", value)

	// Every read the dashboard performs.
	reads := []string{
		"/projects",
		"/projects/" + project.ID,
		"/projects/" + project.ID + "/environments",
		"/environments/" + project.DevelopmentID + "/secrets",
		"/secrets/" + secretID + "/versions",
		"/projects/" + project.ID + "/access",
		"/projects/" + project.ID + "/tokens",
		"/projects/" + project.ID + "/audit",
		"/audit",
		"/identities",
		"/members",
		"/members/invitations",
		"/auth/session",
	}

	for _, path := range reads {
		t.Run(path, func(t *testing.T) {
			resp := h.MustAdmin(http.StatusOK, apitest.Request{
				Path:       path,
				Credential: org.BrowserSession,
			})
			if strings.Contains(resp.Raw, value) {
				t.Fatalf("%s returned a plaintext secret value: %s", path, resp.Raw)
			}
		})
	}

	// And rotation, which handles a value on the way in, must not echo it back.
	rotated := h.MustAdmin(http.StatusOK, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/secrets/" + secretID + "/rotate",
		Credential: org.BrowserSession,
		Body:       map[string]string{"value": "rotated-canary-value-not-real"},
	})
	if strings.Contains(rotated.Raw, "rotated-canary-value-not-real") {
		t.Fatalf("rotation echoed the new value back to the browser: %s", rotated.Raw)
	}
}

// Reveal is the one path that returns plaintext, and it requires a capability
// no role grants. This test walks the full sequence: refused, granted, allowed,
// audited.
func TestRevealRequiresAnExplicitGrantAndIsAudited(t *testing.T) {
	h := apitest.New(t)

	const value = "sk_live_reveal_canary_value"

	org := h.NewOrganization()
	project := h.NewProject(org)
	secretID := h.NewSecret(org, project.DevelopmentID, "STRIPE_SECRET_KEY", value)

	// The owner created it, and still cannot see it.
	refused := h.AdminCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/secrets/" + secretID + "/reveal",
		Credential: org.BrowserSession,
	})
	if refused.Status != http.StatusForbidden {
		t.Fatalf("expected reveal to be refused without READ_SECRET, got %d: %s", refused.Status, refused.Raw)
	}
	if strings.Contains(refused.Raw, value) {
		t.Fatal("the refusal contained the value")
	}

	// The owner grants themselves READ_SECRET. This is permitted; they hold
	// MANAGE_ACCESS, and it is exactly the act the audit trail exists to
	// record.
	h.MustAdmin(http.StatusCreated, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/projects/" + project.ID + "/access",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"subjectType":   "USER",
			"subjectId":     org.UserID,
			"environmentId": project.DevelopmentID,
			"capabilities":  []string{"READ_SECRET"},
			"reason":        "Investigating a failed deployment.",
		},
	})

	revealed := h.MustAdmin(http.StatusOK, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/secrets/" + secretID + "/reveal",
		Credential: org.BrowserSession,
	})
	if revealed.String("value") != value {
		t.Fatalf("reveal did not return the value: %s", revealed.Raw)
	}

	// Both the request and the disclosure are recorded, along with the reason
	// the grant was created and the fact that it was self-granted.
	auditResp := h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/projects/" + project.ID + "/audit?limit=200",
		Credential: org.BrowserSession,
	})

	events, _ := auditResp.Get("events").([]any)
	var requested, disclosed, granted, selfGranted bool
	for _, entry := range events {
		event, _ := entry.(map[string]any)
		switch event["eventType"] {
		case "SECRET_REVEAL_REQUESTED":
			requested = true
		case "SECRET_REVEALED":
			if event["outcome"] == "SUCCESS" {
				disclosed = true
			}
		case "ACCESS_GRANTED":
			granted = true
			if metadata, ok := event["metadata"].(map[string]any); ok {
				if metadata["grants_plaintext_visibility"] == true && metadata["self_granted"] == true {
					selfGranted = true
				}
			}
		}
	}

	if !requested || !disclosed {
		t.Fatalf("the reveal was not fully audited (requested=%v disclosed=%v)", requested, disclosed)
	}
	if !granted || !selfGranted {
		t.Fatal("a self-granted plaintext-visibility grant was not clearly recorded")
	}
	if strings.Contains(auditResp.Raw, value) {
		t.Fatal("the audit trail contains the revealed value")
	}
	if !strings.Contains(auditResp.Raw, "Investigating a failed deployment.") {
		t.Fatal("the stated reason for the grant should be recorded")
	}

	// The value must not have reached the logs on the way out.
	if strings.Contains(h.Logs.String(), value) {
		t.Fatal("revealing a secret wrote it to the logs")
	}
}

// An unauthenticated or wrongly authenticated caller must never receive a value
// on any surface.
func TestUnauthorizedResponsesCarryNoValue(t *testing.T) {
	h := apitest.New(t)

	const value = "unauthorized-canary-value-not-real"

	org := h.NewOrganization()
	project := h.NewProject(org)
	secretID := h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", value)

	cases := []struct {
		name       string
		credential string
	}{
		{"no credential", ""},
		{"nonsense credential", "not-a-credential"},
		{"well-formed but unknown", "vlt_AAAAAAAAAA_" + strings.Repeat("B", 52)},
		{"a credential from another kind", org.BrowserSession},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Runtime delivery.
			runtime := h.RuntimeCall(apitest.Request{
				Method:     http.MethodPost,
				Path:       "/runtime/secrets",
				Credential: tc.credential,
				Body:       map[string]any{"keys": []string{"DATABASE_URL"}},
			})
			if runtime.Status == http.StatusOK {
				t.Fatalf("unauthenticated delivery succeeded: %s", runtime.Raw)
			}
			if strings.Contains(runtime.Raw, value) {
				t.Fatal("an unauthorized response contained the value")
			}

			// Human reveal.
			reveal := h.AdminCall(apitest.Request{
				Method:     http.MethodPost,
				Path:       "/secrets/" + secretID + "/reveal",
				Credential: tc.credential,
			})
			if reveal.Status == http.StatusOK {
				t.Fatalf("unauthorized reveal succeeded: %s", reveal.Raw)
			}
			if strings.Contains(reveal.Raw, value) {
				t.Fatal("an unauthorized reveal contained the value")
			}
		})
	}
}

// The BFF boundary: a caller holding a valid browser session but no service
// token must not be able to use the core API. This is what stops a browser that
// has learned the internal address from talking to it directly.
func TestCoreAPIRequiresTheServiceCredential(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)

	// With the service token, the same request works.
	h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/projects/" + project.ID,
		Credential: org.BrowserSession,
	})

	// Without it, a valid session is not enough.
	withoutToken := h.AdminCall(apitest.Request{
		Path:             "/projects/" + project.ID,
		Credential:       org.BrowserSession,
		OmitServiceToken: true,
	})
	if withoutToken.Status != http.StatusUnauthorized {
		t.Fatalf("the core API served a request with no service credential: %d %s",
			withoutToken.Status, withoutToken.Raw)
	}

	// A wrong one is no better.
	wrongToken := h.AdminCall(apitest.Request{
		Path:                 "/projects/" + project.ID,
		Credential:           org.BrowserSession,
		ServiceTokenOverride: "wrong-service-token-that-is-long-enough-x",
	})
	if wrongToken.Status != http.StatusUnauthorized {
		t.Fatalf("the core API accepted an incorrect service credential: %d", wrongToken.Status)
	}
}

// Browser sessions and CLI logins are both user sessions, and each is refused
// on the other's surface.
func TestSessionKindsAreNotInterchangeable(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "kind-canary-value-not-real")

	// A browser session cannot reach secret delivery. This matters because the
	// browser session is the credential most exposed to a scripting flaw in the
	// dashboard.
	browserOnRuntime := h.RuntimeCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/runtime/auth",
		Credential: org.BrowserSession,
		Body:       map[string]string{"project": project.Slug, "environment": "development"},
	})
	if browserOnRuntime.Status != http.StatusUnauthorized {
		t.Fatalf("a browser session authenticated against the runtime API: %d %s",
			browserOnRuntime.Status, browserOnRuntime.Raw)
	}

	// A CLI login cannot administer the organization.
	cliOnAdmin := h.AdminCall(apitest.Request{
		Path:       "/projects",
		Credential: org.CLISession,
	})
	if cliOnAdmin.Status != http.StatusUnauthorized {
		t.Fatalf("a CLI login authenticated against the dashboard API: %d %s",
			cliOnAdmin.Status, cliOnAdmin.Raw)
	}

	// A machine token cannot administer anything either.
	identityID := h.NewIdentity(org, apitest.Unique("bot"), "CI")
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	machineOnAdmin := h.AdminCall(apitest.Request{
		Path:       "/projects",
		Credential: token.Secret,
	})
	if machineOnAdmin.Status != http.StatusUnauthorized {
		t.Fatalf("a machine token reached the dashboard API: %d %s",
			machineOnAdmin.Status, machineOnAdmin.Raw)
	}
}

// Responses that describe or carry credentials must not be storable by a cache
// anywhere on the path.
func TestResponsesAreNotCacheable(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)

	resp, err := h.Admin.Client().Do(mustRequest(t,
		http.MethodGet, h.Admin.URL+"/environments/"+project.DevelopmentID+"/secrets",
		org.BrowserSession))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	cacheControl := resp.Header.Get("Cache-Control")
	if !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("secret metadata was served without no-store: %q", cacheControl)
	}
	for header, expected := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := resp.Header.Get(header); got != expected {
			t.Errorf("%s = %q, want %q", header, got, expected)
		}
	}
}

func mustRequest(t *testing.T, method, url, credential string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("X-Service-Token", apitest.ServiceToken)
	return req
}
