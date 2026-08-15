package apitest_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Tobe0504/Warder/internal/apitest"
)

// The value used throughout. It is obviously fake, and it doubles as a canary:
// several tests search logs, errors, and responses for it.
const canaryValue = "postgres://payments:canary-not-a-real-password@db.internal:5432/payments"

// TestVerticalSlice walks the entire product in one test, in the order a real
// deployment would: an organization is created, a project and its environments
// exist, a secret is stored encrypted, a machine identity is granted the right
// to use it, a scoped token is minted, a runtime authenticates and receives the
// value, a child process is started with it in its environment, and the whole
// thing is visible in the audit trail.
//
// If this test passes, the product works. Everything else in this package
// checks that it fails correctly.
func TestVerticalSlice(t *testing.T) {
	h := apitest.New(t)
	ctx := context.Background()

	// --- Organization, project, environments -------------------------------
	org := h.NewOrganization()
	project := h.NewProject(org)

	// --- A secret, encrypted at rest ---------------------------------------
	secretID := h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", canaryValue)

	// The ciphertext must not contain the plaintext, and the metadata table
	// must hold no value at all.
	var ciphertext []byte
	err := h.DB.Pool.QueryRow(ctx, `
		SELECT m.ciphertext
		FROM secret_material.secret_version_material m
		JOIN secret_versions v ON v.id = m.secret_version_id
		WHERE v.secret_id = $1`, secretID).Scan(&ciphertext)
	if err != nil {
		t.Fatalf("reading stored material: %v", err)
	}
	if strings.Contains(string(ciphertext), "canary-not-a-real-password") {
		t.Fatal("the stored ciphertext contains the plaintext")
	}

	// --- A machine identity that may use it, but not see it ----------------
	identityID := h.NewIdentity(org, apitest.Unique("payments-api"), "WORKLOAD")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID,
		[]string{"USE_SECRET"}, nil)

	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID,
		[]string{"USE_SECRET"}, nil)

	if token.Secret == "" || !strings.HasPrefix(token.Secret, "vlt_") {
		t.Fatalf("token was not minted correctly: %q", token.Secret)
	}

	// --- The runtime authenticates and fetches ------------------------------
	accessToken := h.RuntimeSession(token.Secret, "", "")
	if !strings.HasPrefix(accessToken, "vrt_") {
		t.Fatalf("expected a short-lived runtime session, got %q", accessToken)
	}

	delivery := h.FetchSecrets(accessToken, []string{"DATABASE_URL"})
	value, ok := delivery.SecretValue("DATABASE_URL")
	if !ok {
		t.Fatalf("the secret was not delivered: %s", delivery.Raw)
	}
	if value != canaryValue {
		t.Fatal("the delivered value does not match what was stored")
	}

	// The response carries the values and the environment name, and none of the
	// internal identifiers a runtime has no use for.
	for _, leaked := range []string{secretID, project.ID, identityID, token.ID} {
		if strings.Contains(delivery.Raw, leaked) {
			t.Fatalf("the delivery response leaked an internal identifier: %s", leaked)
		}
	}

	// --- The child process receives it -------------------------------------
	//
	// This is the moment the product is built around: a process starts with the
	// credential available to it, and the developer never handled the value.
	script := `if [ "$DATABASE_URL" = "` + canaryValue + `" ]; then echo INJECTED; else echo MISSING; fi`
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Env = append(os.Environ(), "DATABASE_URL="+value)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child process failed: %v", err)
	}
	if strings.TrimSpace(string(output)) != "INJECTED" {
		t.Fatalf("the child process did not receive the secret: %q", output)
	}

	// --- The audit trail records it ----------------------------------------
	auditResp := h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/projects/" + project.ID + "/audit",
		Credential: org.BrowserSession,
	})

	events, _ := auditResp.Get("events").([]any)
	var sawUse, sawCreate bool
	for _, entry := range events {
		event, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		switch event["eventType"] {
		case "SECRET_USED":
			if event["secretKey"] == "DATABASE_URL" && event["outcome"] == "SUCCESS" {
				sawUse = true
				if event["actorType"] != "WORKLOAD" {
					t.Fatalf("expected the workload to be recorded as the actor, got %v", event["actorType"])
				}
			}
		case "SECRET_CREATED":
			if event["secretKey"] == "DATABASE_URL" {
				sawCreate = true
			}
		}
	}
	if !sawCreate {
		t.Fatal("secret creation was not audited")
	}
	if !sawUse {
		t.Fatal("secret use was not audited")
	}

	// The audit trail records the name and never the value.
	if strings.Contains(auditResp.Raw, "canary-not-a-real-password") {
		t.Fatal("the audit trail contains the secret value")
	}
	if !strings.Contains(auditResp.Raw, "DATABASE_URL") {
		t.Fatal("the audit trail should name the secret that was used")
	}

	// --- Nothing reached the logs ------------------------------------------
	if strings.Contains(h.Logs.String(), "canary-not-a-real-password") {
		t.Fatal("the secret value appeared in the application logs")
	}
}

// The dashboard's view of a secret must never carry the value, even for the
// organization owner who created it.
func TestDashboardListingIsMaskedForTheOwner(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "STRIPE_SECRET_KEY", canaryValue)

	listing := h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/environments/" + project.DevelopmentID + "/secrets",
		Credential: org.BrowserSession,
	})

	if strings.Contains(listing.Raw, "canary-not-a-real-password") {
		t.Fatalf("the secret listing contained a plaintext value: %s", listing.Raw)
	}
	if !strings.Contains(listing.Raw, "STRIPE_SECRET_KEY") {
		t.Fatal("the listing should name the secret")
	}
	if !strings.Contains(listing.Raw, "••••••••") {
		t.Fatal("the listing should show a mask in place of the value")
	}

	// The owner holds every management capability, and still cannot reveal:
	// READ_SECRET is not conferred by any role.
	secrets, _ := listing.Get("secrets").([]any)
	if len(secrets) != 1 {
		t.Fatalf("expected one secret, got %d", len(secrets))
	}
	entry := secrets[0].(map[string]any)
	if entry["canReveal"] != false {
		t.Fatal("the owner was offered a reveal control without holding READ_SECRET")
	}
	if entry["canRotate"] != true {
		t.Fatal("the owner should be able to rotate")
	}
}

// Rotation must not change what a runtime asks for. The application keeps
// referring to the same key and receives the new value with no reconfiguration.
func TestRotationIsTransparentToRuntimes(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	secretID := h.NewSecret(org, project.DevelopmentID, "API_KEY", "first-value-not-real")

	identityID := h.NewIdentity(org, apitest.Unique("worker"), "SERVICE")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	first := h.FetchSecrets(h.RuntimeSession(token.Secret, "", ""), []string{"API_KEY"})
	if value, _ := first.SecretValue("API_KEY"); value != "first-value-not-real" {
		t.Fatalf("expected the first value, got %q", value)
	}

	rotated := h.MustAdmin(http.StatusOK, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/secrets/" + secretID + "/rotate",
		Credential: org.BrowserSession,
		Body:       map[string]string{"value": "second-value-not-real"},
	})
	if version, ok := rotated.Get("version").(float64); !ok || version != 2 {
		t.Fatalf("expected version 2 after rotation, got %v", rotated.Get("version"))
	}
	// The response must not claim the upstream credential was rotated.
	if rotated.Get("upstreamRotated") != false {
		t.Fatal("rotation should state plainly that the upstream credential was not changed")
	}

	// Same token, same key, no reconfiguration anywhere.
	second := h.FetchSecrets(h.RuntimeSession(token.Secret, "", ""), []string{"API_KEY"})
	if value, _ := second.SecretValue("API_KEY"); value != "second-value-not-real" {
		t.Fatalf("expected the rotated value, got %q", value)
	}
}

// A developer with USE_SECRET can start a process against development, which is
// the human half of the vertical slice.
func TestDeveloperCanRunWithoutSeeing(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "REDIS_URL", "redis://localhost:6379/0")

	// The project creator is granted USE_SECRET on development automatically,
	// so the CLI login can exchange for a runtime session immediately.
	accessToken := h.RuntimeSession(org.CLISession, project.Slug, "development")

	delivery := h.FetchSecrets(accessToken, []string{"REDIS_URL"})
	if value, ok := delivery.SecretValue("REDIS_URL"); !ok || value != "redis://localhost:6379/0" {
		t.Fatalf("the developer could not use the secret: %s", delivery.Raw)
	}

	// The same person cannot reveal it: USE_SECRET does not imply READ_SECRET.
	reveal := h.AdminCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/secrets/" + h.SecretIDFor(org, project.DevelopmentID, "REDIS_URL") + "/reveal",
		Credential: org.BrowserSession,
	})
	if reveal.Status != http.StatusForbidden {
		t.Fatalf("expected reveal to be refused, got %d: %s", reveal.Status, reveal.Raw)
	}
}
