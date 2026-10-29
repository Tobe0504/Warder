// Package store holds every database access path in the system.
//
// All SQL lives here and is parameterized. No query in this package is built by
// concatenating caller-supplied values, and no other package is permitted to
// open a connection, so the set of statements the application can ever execute
// is exactly the set visible in this directory.
package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps the connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects to PostgreSQL and verifies the connection.
func Open(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// The DSN carries the database password, so the driver's error, which
		// may quote it, is discarded rather than wrapped.
		return nil, fmt.Errorf("store: database connection string is not valid")
	}

	cfg.MaxConns = 16
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute

	// Generous, because a serverless Postgres that has scaled to zero takes
	// several seconds to wake. Ten was enough for a database that is always
	// running and turns a first request after an idle period into "the database
	// is unreachable", which sends whoever reads it looking for a fault that is
	// not there.
	cfg.ConnConfig.ConnectTimeout = 30 * time.Second

	if throughPooler(cfg.ConnConfig.Host) {
		// A transaction-mode pooler hands each transaction a different backend,
		// so a prepared statement named on one is absent on the next and every
		// query after the first fails with "prepared statement does not exist".
		// Describing statements on each execution is the documented way to use
		// pgx through PgBouncer, and it costs one extra round trip per query.
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
		cfg.ConnConfig.StatementCacheCapacity = 0
		cfg.ConnConfig.DescriptionCacheCapacity = 0
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: creating connection pool")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: database is unreachable")
	}

	return &DB{Pool: pool}, nil
}

// throughPooler reports whether a host looks like a connection pooler.
//
// Matched by name because that is all the connection string offers, and the
// alternative is a configuration flag that somebody has to know to set. Neon
// and Supabase both mark their pooled endpoints this way; a direct endpoint
// keeps the faster prepared-statement path.
func throughPooler(host string) bool {
	lowered := strings.ToLower(host)
	for _, marker := range []string{"-pooler.", "pgbouncer", "pooler."} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// Close releases the pool.
func (db *DB) Close() { db.Pool.Close() }
