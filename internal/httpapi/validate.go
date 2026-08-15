package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

const (
	// maxRequestedKeys bounds a runtime request so one call cannot ask for an
	// unbounded number of authorization evaluations.
	maxRequestedKeys = 200

	maxNameLength        = 128
	maxDescriptionLength = 512
	maxReasonLength      = 512
)

var (
	// secretKeyPattern matches the shape of an environment variable name.
	// Constraining it here means a secret key is always safe to place in a
	// process environment and always safe to render, without any escaping
	// decision being made at the point of use.
	secretKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

	// slugPattern matches project and environment slugs, which appear in URLs
	// and in CLI configuration.
	slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)
)

func validSecretKey(key string) bool { return secretKeyPattern.MatchString(key) }
func validSlug(slug string) bool     { return slugPattern.MatchString(slug) }

// decodeJSON reads a request body into v, rejecting anything unexpected.
//
// Unknown fields are refused rather than ignored. A request that sets a field
// this build does not know about is a request whose author believes something
// about the system that is not true, and silently discarding it is how a
// misspelled "expiresAt" becomes a credential that never expires.
func decodeJSON(r *http.Request, v any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		return err
	}
	// A body with trailing content suggests the client and server disagree
	// about the payload.
	if decoder.More() {
		return errors.New("request body contains more than one JSON document")
	}
	return nil
}

// validator accumulates field-level problems.
type validator struct {
	problems map[string]string
}

func newValidator() *validator { return &validator{problems: map[string]string{}} }

func (v *validator) add(field, message string) { v.problems[field] = message }

func (v *validator) ok() bool { return len(v.problems) == 0 }

func (v *validator) err() *APIError { return Validation(v.problems) }

// requireName validates a human-facing name.
func (v *validator) requireName(field, value string) string {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		v.add(field, "This field is required.")
	case len(trimmed) > maxNameLength:
		v.add(field, "This must be 128 characters or fewer.")
	}
	return trimmed
}

// requireSlug validates a URL-facing identifier.
func (v *validator) requireSlug(field, value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if !validSlug(trimmed) {
		v.add(field, "Use lowercase letters, digits, and hyphens.")
	}
	return trimmed
}

// requireSecretKey validates a secret's name.
func (v *validator) requireSecretKey(field, value string) string {
	trimmed := strings.TrimSpace(value)
	if !validSecretKey(trimmed) {
		v.add(field, "Use letters, digits, and underscores, starting with a letter or underscore.")
	}
	return trimmed
}

// optionalText validates a bounded free-text field.
func (v *validator) optionalText(field, value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > limit {
		v.add(field, "This field is too long.")
	}
	return trimmed
}

// requireUUID parses an identifier.
func (v *validator) requireUUID(field, value string) uuid.UUID {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		v.add(field, "This is not a valid identifier.")
		return uuid.Nil
	}
	return parsed
}

// capabilities validates a requested capability set.
//
// Unrecognized capabilities are rejected rather than dropped. Silently
// discarding one would store a grant that means less than its author intended,
// and they would have no way to notice.
func (v *validator) capabilities(field string, values []string) []domain.Capability {
	if len(values) == 0 {
		v.add(field, "Select at least one capability.")
		return nil
	}

	seen := map[domain.Capability]bool{}
	out := make([]domain.Capability, 0, len(values))

	for _, raw := range values {
		c := domain.Capability(strings.ToUpper(strings.TrimSpace(raw)))
		if !domain.ValidCapability(c) {
			v.add(field, "Unrecognized capability: "+string(c))
			return nil
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// futureTime parses an optional expiry.
//
// An expiry already in the past is refused. Accepting it would produce a grant
// or a token that is dead the moment it is created, which reads as a bug to
// whoever set it and would send them looking in the wrong place.
func (v *validator) futureTime(field, value string, now time.Time) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		v.add(field, "Use an RFC 3339 timestamp, for example 2026-09-01T12:00:00Z.")
		return nil
	}
	if !parsed.After(now) {
		v.add(field, "This must be in the future.")
		return nil
	}
	return &parsed
}

// pathUUID extracts and validates a path parameter.
func pathUUID(r *http.Request, name string) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, false
	}
	return parsed, true
}
