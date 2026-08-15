package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/secretvalue"
)

func capture(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return New(Options{JSON: true, Output: &buf, Level: slog.LevelDebug}), &buf
}

func TestSensitiveKeysAreRedacted(t *testing.T) {
	logger, buf := capture(t)

	logger.Info("request",
		"authorization", "Bearer abcdefghijklmnop",
		"cookie", "session=abcdef; other=1",
		"password", "hunter2",
		"database_url", "postgres://user:pw@host/db",
		"api_key", "some-api-key-value",
		"x-service-token", "service-token-value",
	)

	out := buf.String()
	for _, leaked := range []string{"hunter2", "abcdefghijklmnop", "some-api-key-value", "service-token-value"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("log contains %q: %s", leaked, out)
		}
	}
}

// Nested attributes are where redaction usually fails, because the key being
// checked is the group's rather than the leaf's.
func TestNestedAndGroupedKeysAreRedacted(t *testing.T) {
	logger, buf := capture(t)

	logger.Info("inbound",
		slog.Group("request",
			slog.String("path", "/runtime/secrets"),
			slog.Group("headers",
				slog.String("authorization", "Bearer super-secret-value"),
				slog.String("user-agent", "warder-cli/0.1"),
			),
		),
		"request.headers.cookie", "session=leak-me",
	)

	out := buf.String()
	if strings.Contains(out, "super-secret-value") || strings.Contains(out, "leak-me") {
		t.Fatalf("nested credential survived redaction: %s", out)
	}
	if !strings.Contains(out, "/runtime/secrets") || !strings.Contains(out, "warder-cli") {
		t.Fatal("redaction removed non-sensitive context that makes logs useful")
	}
}

// A credential logged under an innocent key, or interpolated into a message,
// must still be caught.
func TestCredentialShapesAreScrubbedAnywhere(t *testing.T) {
	tok, err := credential.Mint(credential.KindMachine)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	logger, buf := capture(t)
	logger.Info("authenticating "+tok.Secret, "note", "token was "+tok.Secret, "detail", tok.Secret)

	out := buf.String()
	if strings.Contains(out, tok.Secret) {
		t.Fatalf("a machine token survived redaction: %s", out)
	}
	if !strings.Contains(out, Placeholder) {
		t.Fatalf("expected a redaction placeholder: %s", out)
	}
}

func TestConnectionStringCredentialsAreScrubbed(t *testing.T) {
	logger, buf := capture(t)
	logger.Error("connection failed", "detail",
		"could not connect to postgres://warder:s3cr3t-p4ss@db.internal:5432/payments")

	out := buf.String()
	if strings.Contains(out, "s3cr3t-p4ss") {
		t.Fatalf("connection string password survived: %s", out)
	}
	// The host is what an operator actually needs from this line.
	if !strings.Contains(out, "db.internal") {
		t.Fatalf("redaction removed the host: %s", out)
	}
}

// The connection URI puts a credential in a URL, and URLs are the single most
// commonly logged kind of string. Both userinfo shapes have to be caught.
func TestConnectionURICredentialsAreScrubbed(t *testing.T) {
	cases := map[string]struct{ line, secret, keep string }{
		"warder url": {
			line:   "connecting with warder://svc-token-abc123xyz@api.internal:8443/production",
			secret: "svc-token-abc123xyz",
			keep:   "api.internal",
		},
		"warder url over plain http": {
			line:   "using warder+insecure://local-dev-token-value@127.0.0.1:8080/development",
			secret: "local-dev-token-value",
			keep:   "127.0.0.1",
		},
		"postgres dsn": {
			line:   "postgres://warder:db-password-here@db.internal:5432/warder",
			secret: "db-password-here",
			keep:   "db.internal",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logger, buf := capture(t)
			logger.Info("startup", "detail", tc.line)

			out := buf.String()
			if strings.Contains(out, tc.secret) {
				t.Fatalf("the credential survived redaction: %s", out)
			}
			if !strings.Contains(out, tc.keep) {
				t.Fatalf("redaction removed the host, which operators need: %s", out)
			}
		})
	}
}

// A URL with no credential in it must survive intact, or redaction would strip
// the ordinary logging that makes an outage diagnosable.
func TestPlainURLsAreNotRedacted(t *testing.T) {
	logger, buf := capture(t)
	logger.Info("calling", "url", "https://api.internal:8443/runtime/secrets")

	if !strings.Contains(buf.String(), "https://api.internal:8443/runtime/secrets") {
		t.Fatalf("a credential-free URL was redacted: %s", buf.String())
	}
}

func TestVendorCredentialShapesAreScrubbed(t *testing.T) {
	samples := map[string]string{
		"stripe":      "sk_live_51ABCdefGHIjklMNOpqrST",
		"github":      "ghp_ABCdefGHIjklMNOpqrSTuvwXYZ0123456",
		"aws":         "AKIAIOSFODNN7EXAMPLE",
		"slack":       "xoxb-1234567890-abcdefghijkl",
		"private key": "-----BEGIN RSA PRIVATE KEY-----",
		"bearer":      "Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature",
	}

	for name, sample := range samples {
		t.Run(name, func(t *testing.T) {
			logger, buf := capture(t)
			logger.Info("third party said", "detail", sample)
			if strings.Contains(buf.String(), sample) {
				t.Fatalf("%s credential survived: %s", name, buf.String())
			}
		})
	}
}

// The first layer of defence should already have handled this, but the logger
// must not depend on it.
func TestSecretValueIsRedactedThroughTheHandler(t *testing.T) {
	logger, buf := capture(t)
	const canary = "sk_live_handler_canary_value"

	logger.Info("delivering", "key", "STRIPE_SECRET_KEY", "payload", secretvalue.NewString(canary))

	if strings.Contains(buf.String(), canary) {
		t.Fatalf("secret value reached the log: %s", buf.String())
	}
}

// Attributes attached with With are applied once at construction, so they need
// redacting on that path too.
func TestWithAttrsAreRedacted(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Options{JSON: true, Output: &buf, Level: slog.LevelDebug}).
		With("authorization", "Bearer persistent-secret").
		With(slog.Group("ctx", slog.String("password", "hunter2")))

	logger.Info("hello")

	out := buf.String()
	if strings.Contains(out, "persistent-secret") || strings.Contains(out, "hunter2") {
		t.Fatalf("attributes attached with With were not redacted: %s", out)
	}
}

// Redaction must not strip the information that makes an audit useful. The key
// name is not the secret.
func TestKeyNamesSurviveRedaction(t *testing.T) {
	logger, buf := capture(t)
	logger.Info("secret used",
		"secret_key", "DATABASE_URL",
		"environment", "production",
		"actor", "payments-api",
	)

	out := buf.String()
	for _, expected := range []string{"DATABASE_URL", "production", "payments-api"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q to survive redaction: %s", expected, out)
		}
	}
}

func TestSensitive(t *testing.T) {
	for _, key := range []string{"Authorization", "AUTHORIZATION", "cookie", "request.headers.cookie", "user_password", "db-dsn"} {
		if !Sensitive(key) {
			t.Fatalf("%q should be treated as sensitive", key)
		}
	}
	for _, key := range []string{"path", "method", "secret_key", "environment", "status"} {
		if Sensitive(key) {
			t.Fatalf("%q should not be treated as sensitive", key)
		}
	}
}
