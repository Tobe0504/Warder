package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

// AuditRepo reads the audit trail. Writing goes through audit.Recorder, which
// scrubs events on the way in; this side is read-only by design.
type AuditRepo struct{ db *DB }

// NewAuditRepo constructs the repository.
func NewAuditRepo(db *DB) *AuditRepo { return &AuditRepo{db: db} }

// AuditRecord is a stored event as presented to an administrator.
type AuditRecord struct {
	ID         uuid.UUID
	OccurredAt time.Time

	Type    audit.EventType
	Outcome audit.Outcome

	ActorType  domain.ActorType
	ActorID    *uuid.UUID
	ActorLabel string

	ProjectID     *uuid.UUID
	EnvironmentID *uuid.UUID
	SecretID      *uuid.UUID
	SecretKey     string
	TokenID       *uuid.UUID

	IPAddress string
	UserAgent string
	Reason    string
	Metadata  map[string]any
}

// AuditFilter narrows an audit query.
type AuditFilter struct {
	ProjectID     *uuid.UUID
	EnvironmentID *uuid.UUID
	SecretID      *uuid.UUID
	EventType     string
	Outcome       string
	Limit         int
	Before        *time.Time
}

// List returns audit events for an organization, newest first.
//
// Filters are applied as bound parameters against a fixed statement. The query
// text does not vary with user input beyond which predicates are enabled, so
// there is no path by which a filter value becomes SQL.
func (r *AuditRepo) List(ctx context.Context, orgID uuid.UUID, f AuditFilter) ([]AuditRecord, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, occurred_at, event_type, outcome, actor_type, actor_id, actor_label,
		       project_id, environment_id, secret_id, secret_key, token_id,
		       host(ip_address), user_agent, reason, metadata
		FROM audit_events
		WHERE organization_id = $1
		  AND ($2::uuid IS NULL OR project_id = $2)
		  AND ($3::uuid IS NULL OR environment_id = $3)
		  AND ($4::uuid IS NULL OR secret_id = $4)
		  AND ($5::text IS NULL OR event_type = $5)
		  AND ($6::text IS NULL OR outcome = $6)
		  AND ($7::timestamptz IS NULL OR occurred_at < $7)
		ORDER BY occurred_at DESC
		LIMIT $8`,
		orgID, f.ProjectID, f.EnvironmentID, f.SecretID,
		nullString(f.EventType), nullString(f.Outcome), f.Before, limit)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var out []AuditRecord
	for rows.Next() {
		var rec AuditRecord
		var eventType, outcome, actorType string
		var ip *string
		var metadata []byte

		if err := rows.Scan(&rec.ID, &rec.OccurredAt, &eventType, &outcome, &actorType,
			&rec.ActorID, &rec.ActorLabel,
			&rec.ProjectID, &rec.EnvironmentID, &rec.SecretID, &rec.SecretKey, &rec.TokenID,
			&ip, &rec.UserAgent, &rec.Reason, &metadata); err != nil {
			return nil, translate(err)
		}

		rec.Type = audit.EventType(eventType)
		rec.Outcome = audit.Outcome(outcome)
		rec.ActorType = domain.ActorType(actorType)
		if ip != nil {
			rec.IPAddress = *ip
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &rec.Metadata)
		}
		out = append(out, rec)
	}
	return out, translate(rows.Err())
}

// LastUsed returns the most recent successful use of a secret, which answers
// "is anything still depending on this?" before an administrator revokes it.
func (r *AuditRepo) LastUsed(ctx context.Context, orgID, secretID uuid.UUID) (*AuditRecord, error) {
	records, err := r.List(ctx, orgID, AuditFilter{
		SecretID:  &secretID,
		EventType: string(audit.EventSecretUsed),
		Outcome:   string(audit.OutcomeSuccess),
		Limit:     1,
	})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrNotFound
	}
	return &records[0], nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
