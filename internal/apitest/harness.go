// Package apitest provides a test harness that runs the real core API against
// a real PostgreSQL database.
//
// The tests built on this are the ones that matter most in this repository:
// they exercise authentication, the policy engine, encryption, delivery, and
// audit together, through HTTP, exactly as a runtime would. A unit test can
// confirm the policy engine denies a request; only this can confirm that the
// denial is actually reached before any ciphertext is decrypted.
package apitest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/authz"
	"github.com/Tobe0504/Warder/internal/config"
	"github.com/Tobe0504/Warder/internal/crypto"
	"github.com/Tobe0504/Warder/internal/httpapi"
	"github.com/Tobe0504/Warder/internal/logging"
	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/store"
	"github.com/google/uuid"
)

// ServiceToken is the BFF credential used by the harness.
const ServiceToken = "test-service-token-at-least-32-characters-long"

// Harness is a running core API backed by a real database.
type Harness struct {
	T *testing.T

	Admin   *httptest.Server
	Runtime *httptest.Server

	DB     *store.DB
	Logs   *bytes.Buffer
	Config *config.Config

	Accounts *store.AccountRepo
	Projects *store.ProjectRepo
	Secrets  *store.SecretRepo
	Machines *store.MachineRepo
	Grants   *store.GrantRepo
	Audit    *store.AuditRepo
}

// New starts a harness, skipping the test when no database is configured.
func New(t *testing.T) *Harness {
	t.Helper()

	dsn := os.Getenv("WARDER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WARDER_TEST_DATABASE_URL is not set; skipping API integration test")
	}

	ctx := context.Background()
	if _, err := store.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(db.Close)

	// Logs are captured so that tests can assert on what did and did not reach
	// them. A test that creates a secret and then greps the log for its value
	// is the only reliable check that redaction is working end to end.
	logs := &bytes.Buffer{}
	logger := logging.New(logging.Options{
		Level:  slog.LevelDebug,
		JSON:   true,
		Output: logs,
	})

	key, err := crypto.GenerateKEK()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	provider, err := crypto.NewLocalKeyringProvider(map[string][]byte{"test": key}, "test")
	if err != nil {
		t.Fatalf("key provider: %v", err)
	}
	encryption := crypto.NewEnvelopeEncryptionService(provider)

	cfg := &config.Config{
		Env:               config.EnvDevelopment,
		DatabaseURL:       dsn,
		ServiceToken:      ServiceToken,
		SessionTTL:        time.Hour,
		CLISessionTTL:     time.Hour,
		RuntimeSessionTTL: 5 * time.Minute,
	}

	accounts := store.NewAccountRepo(db)
	projects := store.NewProjectRepo(db)
	secretRepo := store.NewSecretRepo(db)
	machines := store.NewMachineRepo(db)
	grants := store.NewGrantRepo(db)
	auditRepo := store.NewAuditRepo(db)

	recorder := audit.NewDBRecorder(db.Pool, logger)
	policy := authz.NewEngine(grants, time.Now)

	secretService := secrets.NewService(secrets.Config{
		DB: db, Secrets: secretRepo, Crypto: encryption,
		Policy: policy, Audit: recorder, Logger: logger,
	})

	server := httpapi.New(httpapi.Deps{
		Config: cfg, Logger: logger, DB: db,
		Accounts: accounts, Projects: projects, Machines: machines,
		Grants: grants, Audit: auditRepo, SecretRepo: secretRepo,
		Secrets: secretService, Policy: policy, Recorder: recorder, Crypto: encryption,
	})

	h := &Harness{
		T:       t,
		Admin:   httptest.NewServer(server.AdminHandler()),
		Runtime: httptest.NewServer(server.RuntimeHandler()),
		DB:      db, Logs: logs, Config: cfg,
		Accounts: accounts, Projects: projects, Secrets: secretRepo,
		Machines: machines, Grants: grants, Audit: auditRepo,
	}
	t.Cleanup(h.Admin.Close)
	t.Cleanup(h.Runtime.Close)

	// The API answers every failure with a deliberately uninformative message,
	// which is right for callers and useless for a failing test. The captured
	// server log carries the real cause, so it is printed when a test fails —
	// and only then, so that passing runs stay quiet.
	t.Cleanup(func() {
		if t.Failed() && logs.Len() > 0 {
			t.Logf("--- server log ---\n%s", logs.String())
		}
	})

	return h
}

// Response is a decoded API response.
type Response struct {
	Status int
	Body   map[string]any
	Raw    string
}

// Get reads a field from the response body by dotted path.
func (r *Response) Get(path string) any {
	var current any = r.Body
	for _, segment := range strings.Split(path, ".") {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = asMap[segment]
	}
	return current
}

// String reads a string field.
func (r *Response) String(path string) string {
	if v, ok := r.Get(path).(string); ok {
		return v
	}
	return ""
}

// Request describes a call to make.
type Request struct {
	Method string
	Path   string
	Body   any

	// Credential is sent as a bearer token.
	Credential string

	// ServiceToken defaults to the harness value on the admin surface. Set
	// OmitServiceToken to test what happens without it.
	OmitServiceToken     bool
	ServiceTokenOverride string
}

// Admin calls the human-facing API.
func (h *Harness) AdminCall(req Request) *Response {
	h.T.Helper()
	return h.call(h.Admin.URL, req, true)
}

// RuntimeCall calls the machine-facing API.
func (h *Harness) RuntimeCall(req Request) *Response {
	h.T.Helper()
	return h.call(h.Runtime.URL, req, false)
}

func (h *Harness) call(baseURL string, req Request, isAdmin bool) *Response {
	h.T.Helper()

	var body io.Reader
	if req.Body != nil {
		encoded, err := json.Marshal(req.Body)
		if err != nil {
			h.T.Fatalf("encode request: %v", err)
		}
		body = bytes.NewReader(encoded)
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	httpReq, err := http.NewRequest(method, baseURL+req.Path, body)
	if err != nil {
		h.T.Fatalf("build request: %v", err)
	}
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.Credential != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.Credential)
	}
	if isAdmin && !req.OmitServiceToken {
		token := ServiceToken
		if req.ServiceTokenOverride != "" {
			token = req.ServiceTokenOverride
		}
		httpReq.Header.Set("X-Service-Token", token)
	}

	resp, err := h.Admin.Client().Do(httpReq)
	if err != nil {
		h.T.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.T.Fatalf("read response: %v", err)
	}

	out := &Response{Status: resp.StatusCode, Raw: string(raw)}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.Body)
	}
	return out
}

// MustAdmin calls the admin API and fails unless the status matches.
func (h *Harness) MustAdmin(expected int, req Request) *Response {
	h.T.Helper()
	resp := h.AdminCall(req)
	if resp.Status != expected {
		h.T.Fatalf("%s %s: expected %d, got %d: %s", req.Method, req.Path, expected, resp.Status, resp.Raw)
	}
	return resp
}

// MustRuntime calls the runtime API and fails unless the status matches.
func (h *Harness) MustRuntime(expected int, req Request) *Response {
	h.T.Helper()
	resp := h.RuntimeCall(req)
	if resp.Status != expected {
		h.T.Fatalf("%s %s: expected %d, got %d: %s", req.Method, req.Path, expected, resp.Status, resp.Raw)
	}
	return resp
}

// Unique returns a value that will not collide with other test runs.
func Unique(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, strings.ToLower(uuid.NewString()[:8]))
}
