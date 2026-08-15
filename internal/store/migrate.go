package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"

	"github.com/Tobe0504/Warder/migrations"
	"github.com/jackc/pgx/v5"
)

// migrationLockID is an arbitrary constant used for a PostgreSQL advisory lock,
// so that two instances starting at once cannot apply the same migration twice.
const migrationLockID int64 = 0x5741_5244_4552 // "WARDER"

// Migrate applies every pending migration in lexical filename order.
//
// Each migration runs inside its own transaction, so a failure leaves the
// schema at the last complete step rather than half-applied. The checksum of
// each applied file is recorded and re-verified: a migration that has been
// edited after being applied somewhere is a schema drift bug, and it is
// reported rather than silently ignored.
func Migrate(ctx context.Context, dsn string) (applied []string, err error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("migrate: database is unreachable")
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return nil, fmt.Errorf("migrate: acquiring advisory lock: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, migrationLockID); unlockErr != nil && err == nil {
			err = fmt.Errorf("migrate: releasing advisory lock: %w", unlockErr)
		}
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename    text PRIMARY KEY,
			checksum    text        NOT NULL,
			applied_at  timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return nil, fmt.Errorf("migrate: creating migration table: %w", err)
	}

	recorded := map[string]string{}
	rows, err := conn.Query(ctx, `SELECT filename, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: reading applied migrations: %w", err)
	}
	for rows.Next() {
		var filename, checksum string
		if err := rows.Scan(&filename, &checksum); err != nil {
			rows.Close()
			return nil, fmt.Errorf("migrate: reading applied migrations: %w", err)
		}
		recorded[filename] = checksum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: reading applied migrations: %w", err)
	}

	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("migrate: listing migrations: %w", err)
	}
	sort.Strings(entries)

	for _, name := range entries {
		body, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			return nil, fmt.Errorf("migrate: reading %s: %w", name, err)
		}
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])

		if existing, done := recorded[name]; done {
			if existing != checksum {
				return nil, fmt.Errorf(
					"migrate: %s was modified after it was applied; "+
						"add a new migration instead of editing an applied one", name)
			}
			continue
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("migrate: starting transaction for %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return applied, fmt.Errorf("migrate: applying %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)`,
			name, checksum,
		); err != nil {
			_ = tx.Rollback(ctx)
			return applied, fmt.Errorf("migrate: recording %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return applied, fmt.Errorf("migrate: committing %s: %w", name, err)
		}

		applied = append(applied, name)
	}

	return applied, nil
}
