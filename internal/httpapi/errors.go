package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Tobe0504/Warder/internal/crypto"
	"github.com/Tobe0504/Warder/internal/identity"
	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/store"
)

// APIError is the only error shape this API emits.
//
// Every response is built from a fixed set of messages defined below. No
// internal error string is ever interpolated into one, because internal errors
// quote the values they were handling — a failed query quotes its parameters, a
// failed connection quotes its DSN — and for this system those values are
// credentials.
type APIError struct {
	// Code is a stable identifier clients can branch on.
	Code string `json:"code"`
	// Message is safe to display to a user.
	Message string `json:"message"`
	// Details carries field-level validation feedback, never internal state.
	Details map[string]string `json:"details,omitempty"`
}

// Error implements error.
func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// The catalogue of responses. Adding a case means adding it here, which keeps
// the set of things this API can say to an unauthenticated caller reviewable in
// one place.
var (
	ErrBadRequest   = &APIError{Code: "bad_request", Message: "The request could not be understood."}
	ErrUnauthorized = &APIError{Code: "unauthorized", Message: "Authentication is required."}

	// ErrServiceUnauthorized is returned when the caller did not present a
	// valid service credential on the admin surface.
	//
	// It is distinguished from ErrUnauthorized deliberately. Both are 401, and
	// collapsing them means a deployment whose BFF and core API disagree about
	// the service token reports to the person signing in as "your session has
	// ended" — so they sign in again, and again, while the actual fault is in
	// configuration nobody is looking at.
	//
	// The distinction discloses only that a service credential is expected,
	// which is documented and not secret. It says nothing about whether any
	// particular account or session exists.
	ErrServiceUnauthorized = &APIError{
		Code:    "service_unauthorized",
		Message: "This API requires a valid service credential.",
	}
	ErrForbidden    = &APIError{Code: "forbidden", Message: "You do not have access to this resource."}
	ErrNotFound     = &APIError{Code: "not_found", Message: "The resource was not found."}
	ErrConflict     = &APIError{Code: "conflict", Message: "The resource already exists."}
	ErrRateLimited  = &APIError{Code: "rate_limited", Message: "Too many requests. Try again shortly."}
	ErrInternal     = &APIError{Code: "internal_error", Message: "The request could not be completed."}
	ErrPayloadLarge = &APIError{Code: "payload_too_large", Message: "The request body is too large."}

	// ErrSecretUnavailable reports that a secret the caller is authorized for
	// has no usable version. It is only ever returned to a caller who has
	// already passed authorization for that key, so it reveals nothing.
	// ErrAccountExists reports that an address already has an account, so an
	// invitation to it could never be redeemed.
	ErrAccountExists = &APIError{
		Code:    "account_exists",
		Message: "An account already uses that address.",
	}

	// ErrInvitationUnusable is the single answer for every way an invitation
	// can fail to apply: unknown, mistyped, expired, withdrawn, already used.
	//
	// The person redeeming an invitation holds a token and is not the attacker
	// this vagueness defends against — but the endpoint is unauthenticated and
	// therefore reachable by anyone with a guess, and distinguishing "no such
	// invitation" from "that one expired" would make it an oracle for probing
	// which handles exist.
	ErrInvitationUnusable = &APIError{
		Code:    "invitation_unusable",
		Message: "This invitation is not valid. It may have expired, been withdrawn, or already been used. Ask for a new one.",
	}

	ErrSecretUnavailable = &APIError{
		Code:    "secret_unavailable",
		Message: "The secret has no active version. It may have expired or been revoked.",
	}

	// ErrSecretUndecryptable is what a decryption failure looks like from
	// outside. It says nothing about which part failed.
	ErrSecretUndecryptable = &APIError{
		Code:    "secret_unavailable",
		Message: "The secret could not be retrieved.",
	}
)

// Validation builds a field-level error for invalid input.
func Validation(details map[string]string) *APIError {
	return &APIError{
		Code:    "invalid_request",
		Message: "Some fields were not valid.",
		Details: details,
	}
}

func statusFor(err *APIError) int {
	switch err.Code {
	case "bad_request", "invalid_request", "invitation_unusable":
		return http.StatusBadRequest
	case "unauthorized", "service_unauthorized":
		return http.StatusUnauthorized
	case "forbidden":
		return http.StatusForbidden
	case "not_found", "secret_unavailable":
		return http.StatusNotFound
	case "conflict", "account_exists":
		return http.StatusConflict
	case "payload_too_large":
		return http.StatusRequestEntityTooLarge
	case "rate_limited":
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// writeError sends a safe error response and logs the internal cause.
//
// The split is the whole point: the caller learns a category, while the
// operator gets the detail in a log line that has already passed through
// redaction.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, apiErr *APIError, cause error) {
	if apiErr == nil {
		apiErr = ErrInternal
	}

	status := statusFor(apiErr)
	if cause != nil {
		level := slog.LevelWarn
		if status >= 500 {
			level = slog.LevelError
		}
		logger.Log(r.Context(), level, "request failed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"code", apiErr.Code,
			"error", cause.Error(),
		)
	}

	writeJSON(w, r, logger, status, map[string]any{"error": apiErr})
}

// translateError maps a domain or store error onto a safe response.
//
// Anything unrecognized becomes a generic internal error rather than being
// passed through, so a new error type added deep in the stack cannot start
// leaking its text to clients by default.
func translateError(err error) *APIError {
	switch {
	case err == nil:
		return nil

	case errors.Is(err, secrets.ErrNotAuthorized):
		return ErrForbidden

	case errors.Is(err, identity.ErrUnauthenticated):
		return ErrUnauthorized

	// A resource outside the caller's organization is reported as absent, not
	// as forbidden. "Forbidden" would confirm the identifier names something
	// real, which is enough to map another tenant's structure.
	case errors.Is(err, store.ErrNotFound), errors.Is(err, secrets.ErrNotFound):
		return ErrNotFound

	case errors.Is(err, store.ErrConflict):
		return ErrConflict

	case errors.Is(err, secrets.ErrSecretUnavailable):
		return ErrSecretUnavailable

	case errors.Is(err, crypto.ErrDecryptionFailed), errors.Is(err, crypto.ErrKeyUnavailable):
		return ErrSecretUndecryptable

	case errors.Is(err, secrets.ErrValueTooLarge):
		return ErrPayloadLarge

	default:
		return ErrInternal
	}
}

// writeJSON sends a response body.
//
// If encoding fails — which is what happens when a handler accidentally
// includes a secretvalue.Value, since that type refuses to marshal — the
// partial body is discarded and the request fails. A 500 is the correct outcome
// there: the alternative is a successful response carrying plaintext.
func writeJSON(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		logger.Error("response could not be encoded",
			"method", r.Method, "path", r.URL.Path, "error", err.Error())
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"The request could not be completed."}}`))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}
