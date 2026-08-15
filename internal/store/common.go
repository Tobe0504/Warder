package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNotFound is returned when a row does not exist, or exists outside the
// caller's organization. Callers translate it into a 404 without distinguishing
// the two cases, so probing for identifiers reveals nothing.
var ErrNotFound = errors.New("store: not found")

// ErrConflict is returned when a uniqueness constraint rejects a write.
var ErrConflict = errors.New("store: conflict")

// Queryer is satisfied by both the pool and a transaction, so repository
// methods can be composed inside a transaction without duplication.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// translate converts driver errors into the package's sentinels. Driver errors
// are not wrapped: they can quote the offending values, which for this system
// may be credential material.
func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return ErrConflict
		case "23503": // foreign_key_violation
			return ErrNotFound
		}
	}
	return err
}

// InTx runs fn inside a transaction, rolling back on error or panic.
//
// Operations that write both a state change and its audit event use this, so
// the two cannot come apart: an action that happened but was not recorded, or a
// record of one that did not, would each undermine the audit trail.
func InTx(ctx context.Context, db *DB, fn func(tx pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return translate(err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return translate(err)
	}
	committed = true
	return nil
}
