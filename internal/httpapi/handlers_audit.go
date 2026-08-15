package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
	"github.com/google/uuid"
)

// handleProjectAudit returns the audit trail for one project.
func (s *Server) handleProjectAudit(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadAudit, authzTarget{ProjectID: projectID}) {
		return
	}
	if _, err := s.projects.GetProject(r.Context(), principal.OrganizationID, projectID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	filter := s.auditFilter(r)
	filter.ProjectID = &projectID
	s.writeAudit(w, r, principal.OrganizationID, filter)
}

// handleOrganizationAudit returns the organization-wide audit trail.
func (s *Server) handleOrganizationAudit(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !s.allow(w, r, principal, domain.CapReadAudit, authzTarget{}) {
		return
	}
	s.writeAudit(w, r, principal.OrganizationID, s.auditFilter(r))
}

// auditFilter reads the query parameters.
//
// Every value is parsed into a typed field and passed as a bound parameter.
// Anything unparseable is dropped rather than forwarded, so a filter value
// never becomes part of a statement.
func (s *Server) auditFilter(r *http.Request) store.AuditFilter {
	query := r.URL.Query()
	filter := store.AuditFilter{Limit: 100}

	if raw := query.Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			filter.Limit = parsed
		}
	}
	if raw := query.Get("environmentId"); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			filter.EnvironmentID = &parsed
		}
	}
	if raw := query.Get("secretId"); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			filter.SecretID = &parsed
		}
	}
	if raw := query.Get("before"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.Before = &parsed
		}
	}

	// Event type and outcome are constrained to a known shape rather than
	// passed through as free text.
	if raw := strings.ToUpper(strings.TrimSpace(query.Get("eventType"))); raw != "" {
		if isUpperSnake(raw) {
			filter.EventType = raw
		}
	}
	switch strings.ToUpper(strings.TrimSpace(query.Get("outcome"))) {
	case "SUCCESS":
		filter.Outcome = "SUCCESS"
	case "DENIED":
		filter.Outcome = "DENIED"
	case "FAILURE":
		filter.Outcome = "FAILURE"
	}

	return filter
}

func isUpperSnake(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r < 'A' || r > 'Z') && r != '_' {
			return false
		}
	}
	return true
}

func (s *Server) writeAudit(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, filter store.AuditFilter) {
	records, err := s.auditLog.List(r.Context(), orgID, filter)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		out = append(out, map[string]any{
			"id":         rec.ID.String(),
			"occurredAt": rec.OccurredAt.UTC().Format(time.RFC3339),
			"eventType":  string(rec.Type),
			"outcome":    string(rec.Outcome),
			"actorType":  string(rec.ActorType),
			"actor":      rec.ActorLabel,
			// The secret's name, never its value. This is the distinction the
			// whole audit design turns on.
			"secretKey":     rec.SecretKey,
			"secretId":      uuidString(rec.SecretID),
			"projectId":     uuidString(rec.ProjectID),
			"environmentId": uuidString(rec.EnvironmentID),
			"ipAddress":     rec.IPAddress,
			"reason":        rec.Reason,
			"metadata":      rec.Metadata,
		})
	}

	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"events": out})
}
