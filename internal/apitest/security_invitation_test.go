package apitest

import (
	"net/http"
	"strings"
	"sync"
	"testing"
)

// The invitation flow exists to remove one specific weakness: an owner who
// chooses somebody else's password knows their credential, and has to transmit
// it. These tests hold the properties that replaced it.

const inviteePassword = "another-deliberately-fake-passphrase"

// invite issues an invitation and returns its id and token.
func invite(t *testing.T, h *Harness, org *Organization, email, role string) (id, token string) {
	t.Helper()
	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/members/invitations",
		Credential: org.BrowserSession,
		Body:       map[string]string{"email": email, "name": "Invited Person", "role": role},
	})
	return created.String("invitationId"), created.String("token")
}

func TestInvitationAcceptanceCreatesMember(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	email := Unique("invitee") + "@example.test"
	_, token := invite(t, h, org, email, "DEVELOPER")

	if token == "" {
		t.Fatal("no token returned; the invitation cannot be delivered")
	}

	accepted := h.MustAdmin(http.StatusCreated, Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body: map[string]string{
			"token":    token,
			"name":     "Chosen Name",
			"password": inviteePassword,
		},
	})
	if accepted.String("email") != email {
		t.Fatalf("accepted as %q, want %q", accepted.String("email"), email)
	}

	// The invitee can now sign in with a password the inviter never saw.
	browser, _ := h.LoginAs(email, inviteePassword)
	if browser == "" {
		t.Fatal("invitee cannot sign in after accepting")
	}

	members := h.MustAdmin(http.StatusOK, Request{
		Method: http.MethodGet, Path: "/members", Credential: org.BrowserSession,
	})
	if !strings.Contains(members.Raw, email) {
		t.Fatalf("invitee is not in the member list: %s", members.Raw)
	}
}

func TestInvitationIsSingleUse(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	email := Unique("once") + "@example.test"
	_, token := invite(t, h, org, email, "VIEWER")

	h.MustAdmin(http.StatusCreated, Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body:   map[string]string{"token": token, "name": "First", "password": inviteePassword},
	})

	// A second redemption of the same link must fail, or an invitation
	// forwarded to a group chat becomes an unlimited supply of accounts.
	second := h.AdminCall(Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body:   map[string]string{"token": token, "name": "Second", "password": inviteePassword},
	})
	if second.Status == http.StatusCreated {
		t.Fatal("an invitation was redeemed twice")
	}
}

// TestInvitationConcurrentRedemptionCreatesOneAccount drives the race the
// conditional update exists for.
func TestInvitationConcurrentRedemptionCreatesOneAccount(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	email := Unique("race") + "@example.test"
	_, token := invite(t, h, org, email, "DEVELOPER")

	const attempts = 6
	var wg sync.WaitGroup
	statuses := make([]int, attempts)

	for i := range attempts {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			resp := h.AdminCall(Request{
				Method: http.MethodPost,
				Path:   "/auth/accept-invitation",
				Body:   map[string]string{"token": token, "name": "Racer", "password": inviteePassword},
			})
			statuses[slot] = resp.Status
		}(i)
	}
	wg.Wait()

	created := 0
	for _, status := range statuses {
		if status == http.StatusCreated {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("%d concurrent redemptions succeeded, want exactly 1: %v", created, statuses)
	}
}

func TestInvitationRevocationStopsAcceptance(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	email := Unique("revoked") + "@example.test"
	id, token := invite(t, h, org, email, "DEVELOPER")

	h.MustAdmin(http.StatusOK, Request{
		Method:     http.MethodDelete,
		Path:       "/members/invitations/" + id,
		Credential: org.BrowserSession,
	})

	rejected := h.AdminCall(Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body:   map[string]string{"token": token, "name": "Too Late", "password": inviteePassword},
	})
	if rejected.Status == http.StatusCreated {
		t.Fatal("a withdrawn invitation was still accepted")
	}
}

// TestInvitationCannotChooseIdentityOrRole is the property that makes the token
// safe to hand out: it authorizes joining as exactly who the inviter said, at
// exactly the role the inviter chose.
func TestInvitationCannotChooseIdentityOrRole(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	invited := Unique("viewer") + "@example.test"
	_, token := invite(t, h, org, invited, "VIEWER")

	attacker := Unique("attacker") + "@example.test"

	// Two defences, and the outer one fires first: the decoder rejects fields
	// the request type does not declare, so an attempt to smuggle an address or
	// a role in never reaches the handler at all.
	smuggled := h.AdminCall(Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body: map[string]string{
			"token":    token,
			"name":     "Opportunist",
			"password": inviteePassword,
			"email":    attacker,
			"role":     "OWNER",
		},
	})
	if smuggled.Status != http.StatusBadRequest {
		t.Fatalf("acceptance accepted undeclared fields: %d %s", smuggled.Status, smuggled.Raw)
	}

	// The inner defence is what actually holds: even a well-formed acceptance
	// takes its address and role from the invitation row, never the request.
	h.MustAdmin(http.StatusCreated, Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body: map[string]string{
			"token":    token,
			"name":     "Opportunist",
			"password": inviteePassword,
		},
	})

	members := h.MustAdmin(http.StatusOK, Request{
		Method: http.MethodGet, Path: "/members", Credential: org.BrowserSession,
	})
	if strings.Contains(members.Raw, attacker) {
		t.Fatalf("acceptance honoured an attacker-supplied address: %s", members.Raw)
	}
	if !strings.Contains(members.Raw, invited) {
		t.Fatalf("the invited address did not become a member: %s", members.Raw)
	}
	// The role must be the invited one. A second OWNER would mean the request
	// had promoted the invitee.
	if strings.Count(members.Raw, "OWNER") != 1 {
		t.Fatalf("acceptance changed the granted role: %s", members.Raw)
	}
	if !strings.Contains(members.Raw, "VIEWER") {
		t.Fatalf("the invited role was not applied: %s", members.Raw)
	}
}

func TestInvitationRejectsForgedToken(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	email := Unique("forged") + "@example.test"
	_, token := invite(t, h, org, email, "DEVELOPER")

	// Same public handle, different secret half: the row is found and the
	// verifier comparison is what has to refuse it.
	parts := strings.Split(token, "_")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %d parts", len(parts))
	}
	forged := parts[0] + "_" + parts[1] + "_" + strings.Repeat("A", len(parts[2]))

	resp := h.AdminCall(Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body:   map[string]string{"token": forged, "name": "Forger", "password": inviteePassword},
	})
	if resp.Status == http.StatusCreated {
		t.Fatal("a token with the wrong secret half was accepted")
	}
}

// TestInvitationTokenNeverLeaves checks the token is not retrievable after the
// one response that carries it.
func TestInvitationTokenNeverLeaves(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	email := Unique("listing") + "@example.test"
	_, token := invite(t, h, org, email, "DEVELOPER")

	listed := h.MustAdmin(http.StatusOK, Request{
		Method: http.MethodGet, Path: "/members/invitations", Credential: org.BrowserSession,
	})
	if strings.Contains(listed.Raw, token) {
		t.Fatal("the invitation listing returned a redeemable token")
	}

	// Nor may it appear in the server's own log.
	if strings.Contains(h.Logs.String(), token) {
		t.Fatal("an invitation token was written to the server log")
	}
}

// TestInvitationIsScopedToItsOrganization holds the tenancy boundary: another
// organization's owner cannot see or withdraw an invitation that is not theirs.
func TestInvitationIsScopedToItsOrganization(t *testing.T) {
	h := New(t)
	mine := h.NewOrganization()
	theirs := h.NewOrganization()

	email := Unique("tenancy") + "@example.test"
	id, _ := invite(t, h, mine, email, "DEVELOPER")

	listed := h.MustAdmin(http.StatusOK, Request{
		Method: http.MethodGet, Path: "/members/invitations", Credential: theirs.BrowserSession,
	})
	if strings.Contains(listed.Raw, email) {
		t.Fatalf("another organization's invitation was listed: %s", listed.Raw)
	}

	revoked := h.AdminCall(Request{
		Method:     http.MethodDelete,
		Path:       "/members/invitations/" + id,
		Credential: theirs.BrowserSession,
	})
	if revoked.Status == http.StatusOK {
		t.Fatal("another organization's invitation was withdrawn")
	}
}

// TestInvitationRequiresOrganizationManagement checks that issuing one is not
// something an ordinary member can do.
func TestInvitationRequiresOrganizationManagement(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	// Bring in a developer the supported way, then act as them.
	email := Unique("dev") + "@example.test"
	_, token := invite(t, h, org, email, "DEVELOPER")
	h.MustAdmin(http.StatusCreated, Request{
		Method: http.MethodPost,
		Path:   "/auth/accept-invitation",
		Body:   map[string]string{"token": token, "name": "Dev", "password": inviteePassword},
	})
	developerSession, _ := h.LoginAs(email, inviteePassword)

	resp := h.AdminCall(Request{
		Method:     http.MethodPost,
		Path:       "/members/invitations",
		Credential: developerSession,
		Body: map[string]string{
			"email": Unique("smuggled") + "@example.test",
			"name":  "Smuggled",
			"role":  "OWNER",
		},
	})
	if resp.Status == http.StatusCreated {
		t.Fatal("a developer issued an organization invitation")
	}
}

// TestInvitationRefusesExistingAccount stops an invitation being issued that
// could never be redeemed.
func TestInvitationRefusesExistingAccount(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()

	resp := h.AdminCall(Request{
		Method:     http.MethodPost,
		Path:       "/members/invitations",
		Credential: org.BrowserSession,
		Body:       map[string]string{"email": org.Email, "name": "Owner Again", "role": "VIEWER"},
	})
	if resp.Status != http.StatusConflict {
		t.Fatalf("inviting an existing account returned %d, want 409", resp.Status)
	}
}
