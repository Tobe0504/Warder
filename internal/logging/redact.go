// Package logging provides the application's structured logger with redaction
// applied centrally, so that not leaking credentials is a property of the
// logging setup rather than a habit every contributor has to maintain.
//
// This is the second of two layers. The first is secretvalue.Value, which
// refuses to render plaintext at all. This layer catches what the first cannot:
// credentials arriving as ordinary strings from headers, connection strings,
// and third-party libraries that log their own configuration.
package logging

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

// Placeholder replaces any redacted value.
const Placeholder = "[redacted]"

// sensitiveKeys are attribute names whose values are never logged, matched
// case-insensitively against the whole key and against its final segment.
//
// The list covers the names these values actually travel under, including the
// ones third-party libraries choose.
var sensitiveKeys = map[string]bool{
	"authorization":        true,
	"proxy-authorization":  true,
	"cookie":               true,
	"set-cookie":           true,
	"x-service-token":      true,
	"x-csrf-token":         true,
	"password":             true,
	"passwd":               true,
	"secret":               true,
	"secret_value":         true,
	"secretvalue":          true,
	"plaintext":            true,
	"value":                true,
	"token":                true,
	"access_token":         true,
	"refresh_token":        true,
	"id_token":             true,
	"api_key":              true,
	"apikey":               true,
	"private_key":          true,
	"privatekey":           true,
	"client_secret":        true,
	"session":              true,
	"session_token":        true,
	"credential":           true,
	"credentials":          true,
	"database_url":         true,
	"dsn":                  true,
	"connection_string":    true,
	"warder_keyring":       true,
	"keyring":              true,
	"encryption_key":       true,
	"data_key":             true,
	"wrapped_data_key":     true,
	"warder_service_token": true,
}

// valuePatterns match credential shapes wherever they appear inside a logged
// string, including in messages assembled before they reached the logger.
var valuePatterns = []*regexp.Regexp{
	// This system's own credentials, in any of their three kinds.
	regexp.MustCompile(`\b(vlt|vrt|vsn)_[A-Z2-7]{10}_[A-Z2-7]{52}\b`),

	// Connection strings carrying inline credentials. Only the userinfo is
	// replaced, so the host and database remain visible for debugging, which
	// is usually the part someone was trying to log anyway.
	//
	// Both userinfo shapes are matched: "user:password@" as in a PostgreSQL
	// DSN, and a bare "credential@" as in Warder's own WARDER_URL, where the
	// whole userinfo component is the service token. Matching only the first
	// would leave the connection URI readable in any log line that quoted it,
	// which is the main risk of putting a credential in a URI at all.
	regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`),
	regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s:@]+@`),

	// Common vendor credential shapes, so a value pasted into a log line by a
	// downstream library is caught.
	regexp.MustCompile(`\bsk_(live|test)_[A-Za-z0-9]{8,}\b`),
	regexp.MustCompile(`\b(gh[pousr]|github_pat)_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bxox[abporsu]-[A-Za-z0-9-]{10,}\b`),
	regexp.MustCompile(`-----BEGIN[ A-Z]*PRIVATE KEY-----`),

	// Bearer and Basic credentials embedded in a header dump.
	regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`),
}

// Sensitive reports whether an attribute key should have its value withheld.
func Sensitive(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if sensitiveKeys[normalized] {
		return true
	}
	// Match the final segment of dotted or nested keys such as
	// "request.headers.authorization".
	if i := strings.LastIndexAny(normalized, "._-"); i >= 0 && i+1 < len(normalized) {
		if sensitiveKeys[normalized[i+1:]] {
			return true
		}
	}
	return false
}

// Scrub replaces anything in s that looks like a credential.
func Scrub(s string) string {
	for _, pattern := range valuePatterns {
		s = pattern.ReplaceAllStringFunc(s, func(match string) string {
			// The connection-string pattern keeps its scheme so the log still
			// says which service was involved.
			if loc := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`).FindString(match); loc != "" {
				return loc + Placeholder + "@"
			}
			return Placeholder
		})
	}
	return s
}

// redactingHandler wraps a slog.Handler and applies redaction to every record
// passing through it, including attributes attached by With and WithGroup.
type redactingHandler struct {
	inner slog.Handler
}

// NewHandler wraps a handler so that everything logged through it is redacted.
func NewHandler(inner slog.Handler) slog.Handler {
	return &redactingHandler{inner: inner}
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, Scrub(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cleaned := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		cleaned[i] = redactAttr(a)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(cleaned)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	// Resolve first, so a type implementing slog.LogValuer, including
	// secretvalue.Value: has already replaced itself.
	a.Value = a.Value.Resolve()

	if Sensitive(a.Key) {
		return slog.String(a.Key, Placeholder)
	}

	switch a.Value.Kind() {
	case slog.KindGroup:
		attrs := a.Value.Group()
		cleaned := make([]slog.Attr, len(attrs))
		for i, inner := range attrs {
			cleaned[i] = redactAttr(inner)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(cleaned...)}

	case slog.KindString:
		return slog.String(a.Key, Scrub(a.Value.String()))

	case slog.KindAny:
		// Anything that is not a basic kind is rendered and scrubbed rather
		// than passed through, since its String method is not under our
		// control and may print fields we would have redacted by name.
		return slog.String(a.Key, Scrub(a.Value.String()))
	}

	return a
}
