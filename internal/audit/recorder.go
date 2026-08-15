package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Tobe0504/Warder/internal/logging"
	"github.com/Tobe0504/Warder/internal/secretvalue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Recorder writes audit events.
type Recorder interface {
	// Record writes an event using the pool. A failure to write is logged and
	// swallowed, because an audit write failing must not take down the action
	// it describes for a read-only operation.
	Record(ctx context.Context, ev Event)

	// RecordTx writes an event inside a caller's transaction, so that a state
	// change and its audit record commit together or not at all. State-changing
	// operations use this.
	RecordTx(ctx context.Context, tx pgx.Tx, ev Event) error
}

// Execer is the subset of a database handle the recorder needs. Both the
// connection pool and a transaction satisfy it.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DBRecorder writes events to PostgreSQL.
type DBRecorder struct {
	pool   Execer
	logger *slog.Logger
}

const insertEvent = `
	INSERT INTO audit_events
		(organization_id, occurred_at, event_type, outcome,
		 actor_type, actor_id, actor_label, credential_id,
		 project_id, environment_id, secret_id, secret_key, token_id,
		 ip_address, user_agent, reason, metadata)
	VALUES ($1, COALESCE($2, now()), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

// NewDBRecorder constructs a recorder.
func NewDBRecorder(pool Execer, logger *slog.Logger) *DBRecorder {
	return &DBRecorder{pool: pool, logger: logger}
}

// Record implements Recorder.
func (r *DBRecorder) Record(ctx context.Context, ev Event) {
	args, err := eventArgs(ev)
	if err != nil {
		r.logger.Error("audit event could not be prepared",
			"event_type", string(ev.Type), "error", err)
		return
	}

	// The write uses a context detached from the request, so that a client
	// disconnecting mid-request does not prevent the record of what was already
	// done from being written.
	if _, err := r.pool.Exec(context.WithoutCancel(ctx), insertEvent, args...); err != nil {
		r.logger.Error("audit event could not be written",
			"event_type", string(ev.Type), "outcome", string(ev.Outcome), "error", err)
	}
}

// RecordTx implements Recorder.
func (r *DBRecorder) RecordTx(ctx context.Context, tx pgx.Tx, ev Event) error {
	args, err := eventArgs(ev)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, insertEvent, args...); err != nil {
		return fmt.Errorf("audit: writing event: %w", err)
	}
	return nil
}

func eventArgs(ev Event) ([]any, error) {
	metadata, err := json.Marshal(scrubMetadata(ev.Metadata))
	if err != nil {
		return nil, fmt.Errorf("audit: encoding metadata: %w", err)
	}

	var occurredAt any
	if !ev.OccurredAt.IsZero() {
		occurredAt = ev.OccurredAt
	}

	var ip any
	if ev.IPAddress != "" {
		ip = ev.IPAddress
	}

	return []any{
		ev.OrganizationID, occurredAt, string(ev.Type), string(ev.Outcome),
		string(ev.ActorType), ev.ActorID, logging.Scrub(ev.ActorLabel), ev.CredentialID,
		ev.ProjectID, ev.EnvironmentID, ev.SecretID, ev.SecretKey, ev.TokenID,
		ip, logging.Scrub(ev.UserAgent), logging.Scrub(ev.Reason), metadata,
	}, nil
}

// scrubMetadata removes anything credential-shaped from event metadata.
//
// Metadata is the one open-ended field on an event, so it is the one place
// where a value could slip in, a caller adding a helpful "detail" that happens
// to contain a connection string. Everything that goes in is filtered by key
// name and scanned for credential shapes, and any secretvalue.Value is replaced
// outright.
func scrubMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(in))
	for k, v := range in {
		if logging.Sensitive(k) {
			out[k] = logging.Placeholder
			continue
		}
		out[k] = scrubValue(v)
	}
	return out
}

func scrubValue(v any) any {
	switch typed := v.(type) {
	case secretvalue.Value:
		return logging.Placeholder
	case string:
		return logging.Scrub(typed)
	case []string:
		cleaned := make([]string, len(typed))
		for i, s := range typed {
			cleaned[i] = logging.Scrub(s)
		}
		return cleaned
	case map[string]any:
		return scrubMetadata(typed)
	case []any:
		cleaned := make([]any, len(typed))
		for i, item := range typed {
			cleaned[i] = scrubValue(item)
		}
		return cleaned
	case fmt.Stringer:
		return logging.Scrub(typed.String())
	default:
		return v
	}
}
