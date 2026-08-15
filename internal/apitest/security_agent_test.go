package apitest_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tobe0504/Warder/internal/apitest"
)

// The AI agent scenario, which is the product's motivating example.
//
// An agent is treated as an untrusted identity of its own. It can run the tests
// — which means using development credentials — and it can do nothing else: it
// cannot print a value, cannot reach staging or production, cannot rotate or
// revoke anything, and cannot change who has access. None of that depends on
// the agent behaving well, because none of it is enforced in the agent.
func TestAIAgentCanUseDevelopmentAndNothingElse(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)

	testKeyID := h.NewSecret(org, project.DevelopmentID, "STRIPE_TEST_KEY", "sk_test_agent_canary")
	h.NewSecret(org, project.ProductionID, "STRIPE_SECRET_KEY", "sk_live_agent_must_not_see")

	// An agent session identity, bounded in time from the moment it exists.
	sessionEnds := time.Now().Add(2 * time.Hour)
	agentID := h.MustAdmin(http.StatusCreated, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/identities",
		Credential: org.BrowserSession,
		Body: map[string]string{
			"name":      "claude-code-session-" + apitest.Unique("id"),
			"actorType": "AI_AGENT",
			"expiresAt": sessionEnds.UTC().Format(time.RFC3339),
		},
	}).String("id")

	// Development, use only.
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", agentID, []string{"USE_SECRET"}, &sessionEnds)

	agentToken := h.NewToken(org, project.ID, project.DevelopmentID, agentID, []string{"USE_SECRET"}, nil)
	accessToken := h.RuntimeSession(agentToken.Secret, "", "")

	// It can run the tests.
	delivery := h.FetchSecrets(accessToken, []string{"STRIPE_TEST_KEY"})
	if value, ok := delivery.SecretValue("STRIPE_TEST_KEY"); !ok || value != "sk_test_agent_canary" {
		t.Fatalf("the agent could not use its test credential: %s", delivery.Raw)
	}

	// It cannot see production, whether it asks by name or asks for everything.
	if strings.Contains(delivery.Raw, "sk_live_agent_must_not_see") {
		t.Fatal("a production value reached the agent")
	}

	everything := h.FetchSecrets(accessToken, nil)
	if strings.Contains(everything.Raw, "sk_live_agent_must_not_see") {
		t.Fatal("asking for everything gave the agent production access")
	}

	// It cannot mint itself a production session.
	escalation := h.RuntimeCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/runtime/auth",
		Credential: agentToken.Secret,
		Body:       map[string]string{"project": project.Slug, "environment": "production"},
	})
	if escalation.Status == http.StatusOK {
		t.Fatal("the agent obtained a production runtime session")
	}

	// It cannot reveal, rotate, revoke, or change access — the whole admin
	// surface refuses its credential outright.
	adminAttempts := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/secrets/" + testKeyID + "/reveal", nil},
		{http.MethodPost, "/secrets/" + testKeyID + "/rotate", map[string]string{"value": "agent-rotated"}},
		{http.MethodPost, "/projects/" + project.ID + "/access", map[string]any{
			"subjectType": "MACHINE", "subjectId": agentID,
			"environmentId": project.ProductionID, "capabilities": []string{"USE_SECRET"},
		}},
		{http.MethodPost, "/projects/" + project.ID + "/tokens", map[string]any{
			"identityId": agentID, "name": "self-issued",
			"environmentId": project.ProductionID, "capabilities": []string{"USE_SECRET"},
		}},
		{http.MethodGet, "/projects/" + project.ID + "/audit", nil},
	}

	for _, attempt := range adminAttempts {
		resp := h.AdminCall(apitest.Request{
			Method:     attempt.method,
			Path:       attempt.path,
			Credential: agentToken.Secret,
			Body:       attempt.body,
		})
		if resp.Status != http.StatusUnauthorized {
			t.Fatalf("%s %s: the agent reached the admin API (%d): %s",
				attempt.method, attempt.path, resp.Status, resp.Raw)
		}
	}

	// And the value it legitimately used never appeared in the logs, which
	// matters most for an agent: its whole session is often transcribed.
	if strings.Contains(h.Logs.String(), "sk_test_agent_canary") {
		t.Fatal("the agent's credential appeared in the logs")
	}
}

// An agent's authority comes from its own grants, never from whoever created
// it. An owner with broad access must not confer that by minting an agent.
func TestAgentDoesNotInheritItsCreatorsAccess(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "inherit-canary-not-real")

	// The owner grants themselves broad access.
	h.MustAdmin(http.StatusCreated, apitest.Request{
		Method:     http.MethodPost,
		Path:       "/projects/" + project.ID + "/access",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"subjectType":   "USER",
			"subjectId":     org.UserID,
			"environmentId": project.DevelopmentID,
			"capabilities":  []string{"USE_SECRET", "READ_SECRET"},
		},
	})

	// The owner then creates an agent and a token for it, but grants the agent
	// nothing.
	agentID := h.NewIdentity(org, apitest.Unique("agent"), "AI_AGENT")
	agentToken := h.NewToken(org, project.ID, project.DevelopmentID, agentID, []string{"USE_SECRET"}, nil)

	delivery := h.FetchSecrets(h.RuntimeSession(agentToken.Secret, "", ""), nil)
	if _, ok := delivery.SecretValue("DATABASE_URL"); ok {
		t.Fatal("the agent inherited its creator's access")
	}
	if strings.Contains(delivery.Raw, "inherit-canary-not-real") {
		t.Fatal("the agent received a value it was never granted")
	}
}

// The contractor workflow: access ends on its own, and ending it does not
// require rotating any credential.
func TestContractorAccessEndsWithoutRotatingCredentials(t *testing.T) {
	h := apitest.New(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "contractor-canary-not-real")

	// A contractor joins the supported way: invited, then redeeming the link
	// with a password the owner never sees.
	contractor := h.AddMember(org, "DEVELOPER")
	membershipID := contractor.MembershipID
	contractorCLI := contractor.CLISession
	contractorUserID := contractor.UserID

	h.Grant(org, project.ID, project.DevelopmentID, "USER", contractorUserID, []string{"USE_SECRET"}, nil)

	// While engaged, they can run the application.
	working := h.FetchSecrets(h.RuntimeSession(contractorCLI, project.Slug, "development"), nil)
	if _, ok := working.SecretValue("DATABASE_URL"); !ok {
		t.Fatalf("the contractor could not use the development secret: %s", working.Raw)
	}

	// The engagement ends.
	h.MustAdmin(http.StatusOK, apitest.Request{
		Method:     http.MethodDelete,
		Path:       "/members/" + membershipID,
		Credential: org.BrowserSession,
	})

	// Their existing session stops working immediately, not at its expiry.
	ended := h.RuntimeCall(apitest.Request{
		Method:     http.MethodPost,
		Path:       "/runtime/auth",
		Credential: contractorCLI,
		Body:       map[string]string{"project": project.Slug, "environment": "development"},
	})
	if ended.Status != http.StatusUnauthorized {
		t.Fatalf("the contractor still had access after removal: %d %s", ended.Status, ended.Raw)
	}

	// They cannot simply log in again: the password is still correct and now
	// confers nothing.
	relogin := h.RuntimeCall(apitest.Request{
		Method: http.MethodPost,
		Path:   "/cli/login",
		Body: map[string]string{
			"email": contractor.Email, "password": contractor.Password, "kind": "cli",
		},
	})
	if relogin.Status != http.StatusUnauthorized {
		t.Fatalf("a removed contractor logged back in: %d %s", relogin.Status, relogin.Raw)
	}

	// And the credential itself was never touched. This is the point: nobody
	// had to rotate the database password because a contractor left.
	listing := h.MustAdmin(http.StatusOK, apitest.Request{
		Path:       "/environments/" + project.DevelopmentID + "/secrets",
		Credential: org.BrowserSession,
	})
	secrets := mustSlice(listing.Get("secrets"))
	if len(secrets) != 1 {
		t.Fatal("the secret was affected by offboarding")
	}
	if version, _ := secrets[0].(map[string]any)["version"].(float64); version != 1 {
		t.Fatalf("offboarding rotated the credential, which it must not do (version %v)", version)
	}

	// The application that was using it still works.
	identityID := h.NewIdentity(org, apitest.Unique("app"), "WORKLOAD")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	appToken := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	if _, ok := h.FetchSecrets(h.RuntimeSession(appToken.Secret, "", ""), nil).SecretValue("DATABASE_URL"); !ok {
		t.Fatal("offboarding a contractor broke the running application")
	}
}

func mustSlice(v any) []any {
	out, _ := v.([]any)
	return out
}
