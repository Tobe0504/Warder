package apitest

import (
	"net/http"
	"time"
)

// Organization is a bootstrapped tenant with an owner logged in.
type Organization struct {
	ID    string
	Slug  string
	Email string

	// BrowserSession authenticates against the admin API.
	BrowserSession string
	// CLISession authenticates against the runtime API as a human.
	CLISession string
	UserID     string
}

// Project is a created project with its default environments.
type Project struct {
	ID   string
	Slug string

	DevelopmentID string
	StagingID     string
	ProductionID  string
}

// Token is a minted machine credential.
type Token struct {
	ID     string
	Secret string
}

// NewOrganization bootstraps an organization and signs its owner in.
func (h *Harness) NewOrganization() *Organization {
	h.T.Helper()

	slug := Unique("org")
	email := Unique("owner") + "@example.test"
	// A deliberately fake password, long enough to satisfy the policy.
	const password = "correct-horse-battery-staple-test"

	created := h.MustAdmin(http.StatusCreated, Request{
		Method: http.MethodPost,
		Path:   "/organizations",
		Body: map[string]string{
			"organizationName": "Test Organization",
			"slug":             slug,
			"email":            email,
			"name":             "Test Owner",
			"password":         password,
		},
	})

	browser := h.MustAdmin(http.StatusOK, Request{
		Method: http.MethodPost,
		Path:   "/auth/login",
		Body:   map[string]string{"email": email, "password": password, "kind": "browser"},
	})

	cli := h.MustRuntime(http.StatusOK, Request{
		Method: http.MethodPost,
		Path:   "/cli/login",
		Body:   map[string]string{"email": email, "password": password, "kind": "cli"},
	})

	return &Organization{
		ID:             created.String("organizationId"),
		Slug:           slug,
		Email:          email,
		UserID:         created.String("userId"),
		BrowserSession: browser.String("sessionToken"),
		CLISession:     cli.String("sessionToken"),
	}
}

// LoginAs signs an existing user in and returns both session kinds.
func (h *Harness) LoginAs(email, password string) (browserSession, cliSession string) {
	h.T.Helper()

	browser := h.MustAdmin(http.StatusOK, Request{
		Method: http.MethodPost,
		Path:   "/auth/login",
		Body:   map[string]string{"email": email, "password": password, "kind": "browser"},
	})
	cli := h.MustRuntime(http.StatusOK, Request{
		Method: http.MethodPost,
		Path:   "/cli/login",
		Body:   map[string]string{"email": email, "password": password, "kind": "cli"},
	})
	return browser.String("sessionToken"), cli.String("sessionToken")
}

// AddMember brings a person into the organization the supported way: an owner
// issues an invitation, and the invitee redeems it with a password of their own
// choosing.
//
// There is deliberately no shortcut here that creates a member directly. Tests
// exercising membership should go through the same flow real people do, or they
// stop protecting it.
func (h *Harness) AddMember(org *Organization, role string) *NewMember {
	h.T.Helper()

	email := Unique("member") + "@example.test"
	// A deliberately fake password, long enough to satisfy the policy.
	const password = "member-passphrase-for-testing"

	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/members/invitations",
		Credential: org.BrowserSession,
		Body:       map[string]string{"email": email, "name": "Invited Member", "role": role},
	})

	h.MustAdmin(http.StatusCreated, Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body: map[string]string{
			"token":    created.String("token"),
			"name":     "Invited Member",
			"password": password,
		},
	})

	member := &NewMember{Email: email, Password: password}

	members := h.MustAdmin(http.StatusOK, Request{
		Method: http.MethodGet, Path: "/members", Credential: org.BrowserSession,
	})
	list, _ := members.Get("members").([]any)
	for _, entry := range list {
		row, _ := entry.(map[string]any)
		if row["email"] == email {
			member.UserID, _ = row["userId"].(string)
			member.MembershipID, _ = row["membershipId"].(string)
		}
	}
	if member.UserID == "" {
		h.T.Fatalf("the invited member is not in the member list: %s", members.Raw)
	}

	member.BrowserSession, member.CLISession = h.LoginAs(email, password)
	return member
}

// NewMember is a person brought in through the invitation flow.
type NewMember struct {
	Email        string
	Password     string
	UserID       string
	MembershipID string

	BrowserSession string
	CLISession     string
}

// NewProject creates a project and resolves its default environments.
func (h *Harness) NewProject(org *Organization) *Project {
	h.T.Helper()

	slug := Unique("project")
	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/projects",
		Credential: org.BrowserSession,
		Body:       map[string]string{"name": "Payments API", "slug": slug},
	})

	project := &Project{ID: created.String("id"), Slug: slug}

	environments, _ := created.Get("environments").([]any)
	for _, entry := range environments {
		asMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		id, _ := asMap["id"].(string)
		switch asMap["slug"] {
		case "development":
			project.DevelopmentID = id
		case "staging":
			project.StagingID = id
		case "production":
			project.ProductionID = id
		}
	}

	if project.DevelopmentID == "" || project.ProductionID == "" {
		h.T.Fatalf("project was created without its default environments: %s", created.Raw)
	}
	return project
}

// NewSecret stores a secret in an environment.
func (h *Harness) NewSecret(org *Organization, environmentID, key, value string) string {
	h.T.Helper()

	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + environmentID + "/secrets",
		Credential: org.BrowserSession,
		Body:       map[string]string{"key": key, "value": value},
	})
	return created.String("id")
}

// NewIdentity creates a machine identity.
func (h *Harness) NewIdentity(org *Organization, name, actorType string) string {
	h.T.Helper()

	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/identities",
		Credential: org.BrowserSession,
		Body:       map[string]string{"name": name, "actorType": actorType},
	})
	return created.String("id")
}

// Grant creates an access grant for a subject on an environment.
func (h *Harness) Grant(org *Organization, projectID, environmentID, subjectType, subjectID string, capabilities []string, expiresAt *time.Time) string {
	h.T.Helper()

	body := map[string]any{
		"subjectType":   subjectType,
		"subjectId":     subjectID,
		"environmentId": environmentID,
		"capabilities":  capabilities,
	}
	if expiresAt != nil {
		body["expiresAt"] = expiresAt.UTC().Format(time.RFC3339)
	}

	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/projects/" + projectID + "/access",
		Credential: org.BrowserSession,
		Body:       body,
	})
	return created.String("id")
}

// NewToken mints a scoped machine token.
func (h *Harness) NewToken(org *Organization, projectID, environmentID, identityID string, capabilities []string, secretKeys []string) *Token {
	h.T.Helper()

	body := map[string]any{
		"identityId":    identityID,
		"name":          Unique("token"),
		"environmentId": environmentID,
		"capabilities":  capabilities,
	}
	if len(secretKeys) > 0 {
		body["secretKeys"] = secretKeys
	}

	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/projects/" + projectID + "/tokens",
		Credential: org.BrowserSession,
		Body:       body,
	})
	return &Token{ID: created.String("id"), Secret: created.String("token")}
}

// RuntimeSession exchanges a credential for a short-lived runtime session.
func (h *Harness) RuntimeSession(credential, project, environment string) string {
	h.T.Helper()

	body := map[string]string{}
	if project != "" {
		body["project"] = project
	}
	if environment != "" {
		body["environment"] = environment
	}

	resp := h.MustRuntime(http.StatusOK, Request{
		Method:     http.MethodPost,
		Path:       "/runtime/auth",
		Credential: credential,
		Body:       body,
	})
	return resp.String("accessToken")
}

// FetchSecrets retrieves values with a runtime session.
func (h *Harness) FetchSecrets(accessToken string, keys []string) *Response {
	h.T.Helper()

	body := map[string]any{}
	if len(keys) > 0 {
		body["keys"] = keys
	}
	return h.MustRuntime(http.StatusOK, Request{
		Method:     http.MethodPost,
		Path:       "/runtime/secrets",
		Credential: accessToken,
		Body:       body,
	})
}

// SecretIDFor resolves a secret's identifier by key, for tests that need to
// address it directly.
func (h *Harness) SecretIDFor(org *Organization, environmentID, key string) string {
	h.T.Helper()

	listing := h.MustAdmin(http.StatusOK, Request{
		Path:       "/environments/" + environmentID + "/secrets",
		Credential: org.BrowserSession,
	})

	secrets, _ := listing.Get("secrets").([]any)
	for _, entry := range secrets {
		asMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if asMap["key"] == key {
			id, _ := asMap["id"].(string)
			return id
		}
	}
	h.T.Fatalf("no secret named %q in environment %s", key, environmentID)
	return ""
}

// SecretValue reads one delivered value out of a response.
func (r *Response) SecretValue(key string) (string, bool) {
	secrets, ok := r.Get("secrets").(map[string]any)
	if !ok {
		return "", false
	}
	value, ok := secrets[key].(string)
	return value, ok
}
