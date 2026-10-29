package store

import (
	"strings"
	"testing"
)

func TestThroughPooler(t *testing.T) {
	cases := map[string]bool{
		"ep-cool-name-123456-pooler.us-east-2.aws.neon.tech": true,
		"aws-0-eu-west-1.pooler.supabase.com":                true,
		"my-pgbouncer.internal":                              true,
		"ep-cool-name-123456.us-east-2.aws.neon.tech":        false,
		"dpg-abc123-a.oregon-postgres.render.com":            false,
		"127.0.0.1": false,
	}
	for host, want := range cases {
		if got := throughPooler(host); got != want {
			t.Errorf("throughPooler(%q) = %v, want %v", host, got, want)
		}
	}
}

// Migrations take a session-level advisory lock, which a transaction-mode
// pooler silently breaks, so a pooled DSN is refused before anything runs.
func TestMigrateRefusesAPooledConnection(t *testing.T) {
	_, err := Migrate(t.Context(),
		"postgres://u:p@ep-x-123456-pooler.us-east-2.aws.neon.tech/warder")
	if err == nil {
		t.Fatal("a pooled DSN was accepted")
	}
	for _, want := range []string{"connection pooler", "direct connection"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message is missing %q:\n%s", want, err)
		}
	}
}
