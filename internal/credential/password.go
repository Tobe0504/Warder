package credential

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters.
//
// These follow the OWASP Password Storage guidance for Argon2id: 64 MiB of
// memory, three passes, two lanes. Memory cost is what actually frustrates
// GPU and ASIC attacks, so it is the parameter to protect if these are ever
// tuned for latency.
//
// The parameters are stored alongside each hash rather than assumed globally,
// so they can be raised later without invalidating existing passwords: an old
// hash keeps verifying under its own parameters, and can be upgraded on the
// next successful login.
const (
	argonMemoryKiB = 64 * 1024
	argonTime      = 3
	argonThreads   = 2
	argonKeyLen    = 32
	argonSaltLen   = 16
)

// ErrInvalidPasswordHash indicates a stored hash that cannot be parsed. It
// means the record is corrupt or was written by different software; it never
// means the password was wrong.
var ErrInvalidPasswordHash = errors.New("stored password hash is not valid")

// HashPassword derives an Argon2id hash and encodes it in the standard PHC
// string format, the same encoding the reference implementation emits.
//
// Only the encoding is written here. The key derivation itself is
// golang.org/x/crypto/argon2, the Go project's implementation of the algorithm.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("credential: generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks a password against a stored PHC hash in constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	params, salt, want, err := decodePHC(encoded)
	if err != nil {
		return false, err
	}

	got := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NeedsRehash reports whether a stored hash was produced with weaker parameters
// than the current ones, so that a successful login can transparently upgrade
// it. Without this, a parameter increase would only ever protect new accounts.
func NeedsRehash(encoded string) bool {
	params, _, _, err := decodePHC(encoded)
	if err != nil {
		return true
	}
	return params.memory < argonMemoryKiB || params.time < argonTime
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodePHC(encoded string) (argonParams, []byte, []byte, error) {
	var p argonParams

	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, nil, nil, ErrInvalidPasswordHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, nil, nil, ErrInvalidPasswordHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return p, nil, nil, ErrInvalidPasswordHash
	}
	if p.memory == 0 || p.time == 0 || p.threads == 0 {
		return p, nil, nil, ErrInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, ErrInvalidPasswordHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return p, nil, nil, ErrInvalidPasswordHash
	}

	return p, salt, key, nil
}

// DummyVerify performs a throwaway Argon2id derivation.
//
// It is called when a login is attempted for an address that has no account, so
// that the request costs the same as one for an account that does exist.
// Without it, response timing would answer "does this person have an account
// here", which is exactly the question an attacker enumerating a customer list
// is asking.
func DummyVerify(password string) {
	salt := make([]byte, argonSaltLen)
	_ = argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
}
