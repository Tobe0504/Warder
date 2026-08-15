package apitest

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Bulk import exists so that pasting a .env file is one act. These tests hold
// the properties that make it safe to treat it as one.

func TestBulkImportStoresEverySecret(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()
	project := h.NewProject(org)

	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"secrets": []map[string]string{
				{"key": "DATABASE_URL", "value": "bulk-canary-one-not-real"},
				{"key": "STRIPE_SECRET_KEY", "value": "bulk-canary-two-not-real"},
				{"key": "AUTH_SECRET", "value": "bulk-canary-three-not-real"},
			},
		},
	})

	if count, _ := created.Get("count").(float64); count != 3 {
		t.Fatalf("created %v secrets, want 3: %s", created.Get("count"), created.Raw)
	}

	listing := h.MustAdmin(http.StatusOK, Request{
		Path:       "/environments/" + project.DevelopmentID + "/secrets",
		Credential: org.BrowserSession,
	})
	for _, key := range []string{"DATABASE_URL", "STRIPE_SECRET_KEY", "AUTH_SECRET"} {
		if !strings.Contains(listing.Raw, key) {
			t.Fatalf("%s is missing after a bulk import: %s", key, listing.Raw)
		}
	}
}

// TestBulkImportIsAllOrNothing is the reason this endpoint exists rather than
// the browser looping the single-secret one.
func TestBulkImportIsAllOrNothing(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()
	project := h.NewProject(org)

	// The last entry has a key the API will not accept. Nothing before it may
	// survive, or the environment is left holding half a configuration.
	rejected := h.AdminCall(Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"secrets": []map[string]string{
				{"key": "GOOD_ONE", "value": "partial-canary-not-real"},
				{"key": "GOOD_TWO", "value": "partial-canary-not-real"},
				{"key": "9-not-a-valid-key", "value": "partial-canary-not-real"},
			},
		},
	})
	if rejected.Status == http.StatusCreated {
		t.Fatal("a batch with an invalid key was accepted")
	}

	listing := h.MustAdmin(http.StatusOK, Request{
		Path:       "/environments/" + project.DevelopmentID + "/secrets",
		Credential: org.BrowserSession,
	})
	if strings.Contains(listing.Raw, "GOOD_ONE") || strings.Contains(listing.Raw, "GOOD_TWO") {
		t.Fatalf("a rejected batch left secrets behind: %s", listing.Raw)
	}
}

func TestBulkImportRejectsDuplicateKeysWithinTheBatch(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()
	project := h.NewProject(org)

	// Silently letting the last one win would lose a value somebody meant to
	// set, and they would have no way to notice.
	resp := h.AdminCall(Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"secrets": []map[string]string{
				{"key": "SAME_KEY", "value": "first-not-real"},
				{"key": "SAME_KEY", "value": "second-not-real"},
			},
		},
	})
	if resp.Status == http.StatusCreated {
		t.Fatal("a batch naming the same key twice was accepted")
	}
}

func TestBulkImportRefusesKeysTheEnvironmentAlreadyHas(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "existing-canary-not-real")

	resp := h.AdminCall(Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"secrets": []map[string]string{
				{"key": "DATABASE_URL", "value": "overwrite-canary-not-real"},
			},
		},
	})
	if resp.Status == http.StatusCreated {
		t.Fatal("a bulk import overwrote an existing key; that is a rotation, not an add")
	}
}

func TestBulkImportIsBounded(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()
	project := h.NewProject(org)

	entries := make([]map[string]string, 0, 101)
	for i := range 101 {
		entries = append(entries, map[string]string{
			"key":   fmt.Sprintf("KEY_%d", i),
			"value": "bounded-canary-not-real",
		})
	}

	resp := h.AdminCall(Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: org.BrowserSession,
		Body:       map[string]any{"secrets": entries},
	})
	if resp.Status == http.StatusCreated {
		t.Fatal("an unbounded batch was accepted")
	}
}

// TestBulkImportLeaksNoValues holds the same canary rule as every other write
// path: a value goes in and is never echoed back or written to a log.
func TestBulkImportLeaksNoValues(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()
	project := h.NewProject(org)

	const canary = "bulk-leak-canary-not-real"

	created := h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"secrets": []map[string]string{
				{"key": "LEAK_ONE", "value": canary},
				{"key": "LEAK_TWO", "value": canary},
			},
		},
	})
	if strings.Contains(created.Raw, canary) {
		t.Fatalf("the bulk response echoed a value: %s", created.Raw)
	}

	listing := h.MustAdmin(http.StatusOK, Request{
		Path:       "/environments/" + project.DevelopmentID + "/secrets",
		Credential: org.BrowserSession,
	})
	if strings.Contains(listing.Raw, canary) {
		t.Fatal("the secret listing returned a bulk-imported value")
	}

	if strings.Contains(h.Logs.String(), canary) {
		t.Fatal("a bulk-imported value was written to the server log")
	}
}

// TestBulkImportRecordsEachSecretSeparately keeps the audit trail answerable.
func TestBulkImportRecordsEachSecretSeparately(t *testing.T) {
	h := New(t)
	org := h.NewOrganization()
	project := h.NewProject(org)

	h.MustAdmin(http.StatusCreated, Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: org.BrowserSession,
		Body: map[string]any{
			"secrets": []map[string]string{
				{"key": "AUDIT_ONE", "value": "audit-canary-not-real"},
				{"key": "AUDIT_TWO", "value": "audit-canary-not-real"},
			},
		},
	})

	events := h.MustAdmin(http.StatusOK, Request{
		Path:       "/projects/" + project.ID + "/audit",
		Credential: org.BrowserSession,
	})
	// "Two secrets were created" would not answer "when did AUDIT_TWO appear".
	for _, key := range []string{"AUDIT_ONE", "AUDIT_TWO"} {
		if !strings.Contains(events.Raw, key) {
			t.Fatalf("the audit log does not name %s: %s", key, events.Raw)
		}
	}
}

// TestBulkImportIsScopedToTheOrganization keeps the tenancy boundary on the new
// route, which is exactly the sort of place a new endpoint forgets it.
func TestBulkImportIsScopedToTheOrganization(t *testing.T) {
	h := New(t)
	mine := h.NewOrganization()
	theirs := h.NewOrganization()
	project := h.NewProject(mine)

	resp := h.AdminCall(Request{
		Method:     http.MethodPost,
		Path:       "/environments/" + project.DevelopmentID + "/secrets/batch",
		Credential: theirs.BrowserSession,
		Body: map[string]any{
			"secrets": []map[string]string{
				{"key": "SMUGGLED", "value": "tenancy-canary-not-real"},
			},
		},
	})
	if resp.Status == http.StatusCreated {
		t.Fatal("another organization wrote secrets into this environment")
	}

	listing := h.MustAdmin(http.StatusOK, Request{
		Path:       "/environments/" + project.DevelopmentID + "/secrets",
		Credential: mine.BrowserSession,
	})
	if strings.Contains(listing.Raw, "SMUGGLED") {
		t.Fatalf("a cross-tenant bulk write landed: %s", listing.Raw)
	}
}
