package crypto

import (
	"context"
	"errors"
)

// ErrKeyUnavailable is returned when the key material needed to unwrap a data
// key cannot be reached: the key version is unknown to this deployment, the KMS
// is unreachable, or the operator has revoked access.
//
// It is reported to callers as a generic decryption failure. The distinction
// matters operationally — a restored database with the wrong keyring configured
// produces this, not corruption — but it must never reach an API response,
// where "this key version is unknown here" would be an information leak.
var ErrKeyUnavailable = errors.New("crypto: key unavailable")

// KeyProvider is the boundary between the broker and whatever holds the root of
// the key hierarchy. The broker never sees a key encryption key: it hands over
// a data key to be wrapped, and later hands the wrapped bytes back to be
// unwrapped.
//
// That shape is what allows the local development keyring and a cloud HSM to be
// swapped without touching a line of the encryption service, and it is why the
// root key can live somewhere the database and the source tree cannot reach.
type KeyProvider interface {
	// ActiveKeyID names the key that new material must be encrypted under.
	// Existing ciphertext continues to reference the key version it was written
	// with, which is what makes key rotation incremental rather than a
	// stop-the-world re-encryption.
	ActiveKeyID(ctx context.Context) (string, error)

	// Wrap encrypts a data key under the named key encryption key, binding the
	// supplied encryption context. Providers that support a native encryption
	// context (AWS KMS, GCP Cloud KMS) must pass it through rather than
	// ignoring it.
	Wrap(ctx context.Context, keyID string, dataKey []byte, encCtx map[string]string) ([]byte, error)

	// Unwrap reverses Wrap. It must fail when the encryption context differs
	// from the one used at wrap time, and must return ErrKeyUnavailable when
	// keyID is not known to this deployment.
	Unwrap(ctx context.Context, keyID string, wrapped []byte, encCtx map[string]string) ([]byte, error)

	// Describe returns a short, non-sensitive description of the provider for
	// startup logs and health output. It must never include key material.
	Describe() string
}
