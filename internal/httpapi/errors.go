package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Tobe0504/Warder/internal/crypto"
	"github.com/Tobe0504/Warder/internal/identity"
	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/store"
)

// APIError is the only error shape this API emits.
//
// Every response is built from a fixed set of messages defined below. No
// internal error string is ever interpolated into one, because internal errors
// quote the values they were handling, a failed query quotes its parameters, a
// failed connection quotes its DSN, and for this system those values are
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
	// ended", so they sign in again, and again, while the actual fault is in
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
	// ErrAccountExists reports that the invited address already has an account.
	//
	// Not a dead end: the invitation is still good, it simply has to be claimed
	// by the person signing in rather than by creating a second account for the
	// same address. The message says so, because the previous behaviour was to
	// fail on a unique-constraint violation and answer "this invitation is not
	// valid", which was both wrong and unactionable.
	ErrAccountExists = &APIError{
		Code:    "account_exists",
		Message: "You already have a Warder account with this address. Sign in, then open the invitation link again.",
	}

	// ErrIncorrectPassword reports that the current password supplied alongside
	// a change did not match. Named rather than collapsed into unauthorized,
	// because the caller is signed in and the field they got wrong is one they
	// are looking at.
	ErrIncorrectPassword = &APIError{
		Code:    "incorrect_password",
		Message: "That is not your current password.",
	}

	// ErrRecoveryFailed is the single answer for every way a recovery attempt
	// can fail: no such address, wrong code, already-spent code.
	//
	// One answer on purpose. Separating "no such account" from "wrong code"
	// would turn an unauthenticated endpoint into a way to ask whether an
	// address has a Warder account, which is exactly what somebody with a list
	// of addresses would want.
	ErrRecoveryFailed = &APIError{
		Code:    "recovery_failed",
		Message: "That address and recovery code do not match. Each code works once.",
	}

	// ErrRecoveryCodeRejected refuses a recovery code offered in place of a
	// passkey. The password is already known to be right by then, so there is
	// no address to avoid confirming and the message can be specific.
	ErrRecoveryCodeRejected = &APIError{
		Code:    "recovery_code_rejected",
		Message: "That recovery code is not valid, or has already been used.",
	}

	// ErrPasskeysUnavailable reports that this deployment has not been told its
	// own domain, so it cannot verify a credential bound to one.
	ErrPasskeysUnavailable = &APIError{
		Code:    "passkeys_unavailable",
		Message: "Passkeys are not configured on this deployment.",
	}

	// ErrPasskeyRejected is the single answer for every way an assertion or an
	// attestation can fail: a malformed response, a wrong signature, an unknown
	// credential, a counter that did not advance.
	ErrPasskeyRejected = &APIError{
		Code:    "passkey_rejected",
		Message: "That passkey could not be verified. Try again, or use a recovery code.",
	}

	// ErrChallengeExpired reports that the ceremony took too long or was
	// already answered. A challenge is single use, so a second attempt with the
	// same one lands here.
	ErrChallengeExpired = &APIError{
		Code:    "challenge_expired",
		Message: "That took too long. Start again.",
	}

	// ErrSelfRecovery reports an owner issuing a recovery link for their own
	// account, which is a password change they can already make and would turn
	// a borrowed session into a way to lock the real owner out.
	ErrSelfRecovery = &APIError{
		Code:    "self_recovery",
		Message: "Use the change-password screen for your own account.",
	}

	// ErrPasswordlessUnsafe refuses a passkey-only sign-in for an account that
	// has no second way back in.
	//
	// Not a security refusal but a lockout one: with one passkey, no recovery
	// codes and no email, losing the device ends the account permanently. The
	// password still works, so this costs the person a sign-in rather than
	// their access.
	ErrPasswordlessUnsafe = &APIError{
		Code:    "passwordless_unsafe",
		Message: "Add a second passkey or generate recovery codes before signing in without a password. With only one key and no codes, losing it would lock you out for good.",
	}

	// ErrNoOrganization reports that the caller is signed in but belongs to no
	// organization, so there is nothing for this route to act on. Distinct
	// from forbidden, which means "you are in one and may not do this": the
	// answer here is to create an organization or accept an invitation, and
	// the interface can say so.
	ErrNoOrganization = &APIError{
		Code:    "no_organization",
		Message: "You do not belong to an organization yet. Create one, or accept an invitation.",
	}

	// ErrAlreadyMember reports that the invited address is already in this
	// organization, which is the one case where an invitation genuinely has
	// nothing to do.
	ErrAlreadyMember = &APIError{
		Code:    "already_member",
		Message: "That address is already a member of this organization.",
	}

	// ErrInvitationUnusable is the single answer for every way an invitation
	// can fail to apply: unknown, mistyped, expired, withdrawn, already used.
	//
	// The person redeeming an invitation holds a token and is not the attacker
	// this vagueness defends against, but the endpoint is unauthenticated and
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
	case "bad_request", "invalid_request", "invitation_unusable",
		"challenge_expired", "passkeys_unavailable", "passwordless_unsafe":
		return http.StatusBadRequest
	case "unauthorized", "service_unauthorized", "recovery_failed",
		"incorrect_password", "passkey_rejected", "recovery_code_rejected":
		return http.StatusUnauthorized
	case "forbidden", "no_organization", "self_recovery":
		return http.StatusForbidden
	case "not_found", "secret_unavailable":
		return http.StatusNotFound
	case "conflict", "account_exists", "already_member":
		return http.StatusConflict
	case "payload_too_large":
		return http.StatusRequestEntityTooLarge
	case "rate_limited":
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// rateLimited builds the refusal with the wait included in the message.
//
// "Try again shortly" is fine for a limit measured in seconds and misleading
// for one measured in minutes: the reveal allowance refills at ten an hour, so
// somebody who has spent it waits six minutes for the next one and has no way
// to know that from a message telling them to try shortly.
func rateLimited(retryAfter time.Duration) *APIError {
	wait := "a moment"
	switch seconds := int(retryAfter.Seconds()) + 1; {
	case seconds >= 120:
		wait = strconv.Itoa((seconds+59)/60) + " minutes"
	case seconds >= 60:
		wait = "a minute"
	case seconds > 1:
		wait = strconv.Itoa(seconds) + " seconds"
	}

	return &APIError{
		Code:    "rate_limited",
		Message: "Too many requests. Try again in " + wait + ".",
		Details: map[string]string{"retryAfter": strconv.Itoa(int(retryAfter.Seconds()) + 1)},
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
// If encoding fails, which is what happens when a handler accidentally
// includes a secretvalue.Value, since that type refuses to marshal, the
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
