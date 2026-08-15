package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/Tobe0504/Warder/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// testDSN returns the integration test database connection string, or skips.
// Tests that need PostgreSQL skip rather than fail when it is absent, so that
// `go test ./...` stays useful without infrastructure while still running for
// real in CI.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("WARDER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WARDER_TEST_DATABASE_URL is not set; skipping database integration test")
	}
	return dsn
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	applied, err := store.Migrate(ctx, dsn)
	if err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	t.Logf("applied %d migrations", len(applied))

	again, err := store.Migrate(ctx, dsn)
	if err != nil {
		t.Fatalf("second migration run: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("re-running migrations applied %v", again)
	}
}

// The audit trail must resist being rewritten by whatever role the application
// runs as, so that an attacker who reaches the database cannot erase evidence.
func TestAuditEventsAreAppendOnly(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	if _, err := store.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var orgID string
	if err := conn.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ('Append Only Test', $1) RETURNING id`,
		"append-only-test-"+randomSuffix(t),
	).Scan(&orgID); err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	var eventID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO audit_events (organization_id, event_type, outcome, actor_type, actor_label)
		VALUES ($1, 'SECRET_USED', 'SUCCESS', 'WORKLOAD', 'payments-api')
		RETURNING id`, orgID,
	).Scan(&eventID); err != nil {
		t.Fatalf("writing audit event: %v", err)
	}

	if _, err := conn.Exec(ctx,
		`UPDATE audit_events SET outcome = 'DENIED' WHERE id = $1`, eventID); err == nil {
		t.Fatal("an audit event was updated")
	}
	if _, err := conn.Exec(ctx,
		`DELETE FROM audit_events WHERE id = $1`, eventID); err == nil {
		t.Fatal("an audit event was deleted")
	}

	// Clean up through the organization, which cascades.
	if _, err := conn.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID); err != nil {
		t.Logf("cleanup: %v", err)
	}
}

// Two active versions of one secret would make "which value does my application
// receive" ambiguous, so the database refuses it rather than relying on the
// application getting every write path right.
func TestOnlyOneActiveVersionPerSecret(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	if _, err := store.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	suffix := randomSuffix(t)
	var orgID, projectID, envID, secretID string

	if err := conn.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ('Version Test', $1) RETURNING id`,
		"version-test-"+suffix).Scan(&orgID); err != nil {
		t.Fatalf("organization: %v", err)
	}
	defer func() { _, _ = conn.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID) }()

	if err := conn.QueryRow(ctx,
		`INSERT INTO projects (organization_id, name, slug) VALUES ($1, 'Payments', 'payments-api') RETURNING id`,
		orgID).Scan(&projectID); err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := conn.QueryRow(ctx,
		`INSERT INTO environments (project_id, name, slug) VALUES ($1, 'Development', 'development') RETURNING id`,
		projectID).Scan(&envID); err != nil {
		t.Fatalf("environment: %v", err)
	}
	if err := conn.QueryRow(ctx,
		`INSERT INTO secrets (environment_id, key) VALUES ($1, 'DATABASE_URL') RETURNING id`,
		envID).Scan(&secretID); err != nil {
		t.Fatalf("secret: %v", err)
	}

	if _, err := conn.Exec(ctx, `
		INSERT INTO secret_versions (secret_id, version, status, encryption_key_id)
		VALUES ($1, 1, 'ACTIVE', 'local:v1')`, secretID); err != nil {
		t.Fatalf("first version: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO secret_versions (secret_id, version, status, encryption_key_id)
		VALUES ($1, 2, 'ACTIVE', 'local:v1')`, secretID); err == nil {
		t.Fatal("a second active version was accepted")
	}

	// A superseded version alongside an active one is fine; that is rollback.
	if _, err := conn.Exec(ctx, `
		INSERT INTO secret_versions (secret_id, version, status, encryption_key_id)
		VALUES ($1, 2, 'SUPERSEDED', 'local:v1')`, secretID); err != nil {
		t.Fatalf("superseded version alongside active: %v", err)
	}
}

// The secrets table must have no column capable of holding a value, so that a
// dump of it cannot contain one regardless of application bugs.
func TestSecretsTableHasNoValueColumn(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	if _, err := store.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'secrets'`)
	if err != nil {
		t.Fatalf("inspecting columns: %v", err)
	}
	defer rows.Close()

	forbidden := map[string]bool{"value": true, "plaintext": true, "secret_value": true, "ciphertext": true}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if forbidden[name] {
			t.Fatalf("secrets metadata table has a %q column", name)
		}
	}
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}
