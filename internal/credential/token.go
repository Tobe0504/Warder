// Package credential mints and verifies the bearer credentials used by
// machines, CLI logins, and browser sessions.
//
// Every credential in the system has the same shape and the same lifecycle
// rules, so there is one place to audit how they are generated, stored, and
// compared.
package credential

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
)

// Kind identifies what a credential is for. The prefix is visible to whoever
// holds the token, which makes a leaked credential in a log or a paste
// immediately identifiable — and greppable by secret scanners.
type Kind string

const (
	// KindMachine is a long-lived runtime token held by a workload, CI system,
	// or AI agent session.
	KindMachine Kind = "vlt"

	// KindRuntime is the short-lived credential minted by POST /runtime/auth
	// and presented to POST /runtime/secrets.
	KindRuntime Kind = "vrt"

	// KindSession is a browser or CLI login session.
	KindSession Kind = "vsn"

	// KindInvite is a single-use invitation to join an organization. It
	// authenticates nothing on its own: presenting it only allows the holder to
	// create the one account the invitation already names.
	KindInvite Kind = "vin"
)

const (
	// publicIDBytes produces the non-secret lookup handle. It is random rather
	// than derived from the secret, so publishing it in a UI or a log reveals
	// nothing about the secret half.
	publicIDBytes = 6 // 10 base32 characters

	// secretBytes is the unguessable half: 256 bits from the system CSPRNG.
	secretBytes = 32 // 52 base32 characters
)

// Credentials are encoded with unpadded base32 rather than base64.
//
// The encoding alphabet has to exclude the underscore that separates a
// credential's fields, or a credential could encode a character that splits it
// into the wrong pieces. URL-safe base64 includes underscore and standard
// base64 includes slash and padding; base32's alphabet is A-Z and 2-7 only, so
// a credential is always exactly three fields, survives a URL, a shell word, an
// HTTP header, and a copy-paste, and is unambiguous when read aloud.
var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// ErrMalformed is returned when a presented string is not a credential of the
// expected shape. It is deliberately indistinguishable, to the caller, from a
// credential that simply does not exist.
var ErrMalformed = errors.New("credential is malformed")

// Token is a freshly minted credential. The Secret field exists only in memory,
// only on the request that created it, and is shown to its owner exactly once.
type Token struct {
	// Secret is the full credential string, the only form that authenticates.
	Secret string

	// PublicID is the non-secret lookup handle, stored in the clear and shown
	// in listings so a person can identify a credential without holding it.
	PublicID string

	// Hash is the SHA-256 verifier stored in the database.
	Hash []byte
}

// Mint generates a new credential of the given kind.
//
// The result has the form:
//
//	vlt_QK3ZR7XA2M_<52 characters of base32-encoded randomness>
//	    └ public     └ secret
//
// The two halves are independently random. Lookup uses the public half, so
// verification touches exactly one row and never has to scan; the secret half
// is what is actually compared.
func Mint(kind Kind) (*Token, error) {
	publicRaw := make([]byte, publicIDBytes)
	if _, err := rand.Read(publicRaw); err != nil {
		return nil, fmt.Errorf("credential: generating public id: %w", err)
	}
	secretRaw := make([]byte, secretBytes)
	if _, err := rand.Read(secretRaw); err != nil {
		return nil, fmt.Errorf("credential: generating secret: %w", err)
	}

	publicID := encoding.EncodeToString(publicRaw)
	full := fmt.Sprintf("%s_%s_%s", kind, publicID, encoding.EncodeToString(secretRaw))

	return &Token{
		Secret:   full,
		PublicID: publicID,
		Hash:     Hash(full),
	}, nil
}

// Hash returns the stored verifier for a credential.
//
// SHA-256 is the right choice here and a password hash would be the wrong one.
// A password is chosen by a human and has perhaps 30 bits of entropy, so the
// defence has to be making each guess expensive. These credentials carry 256
// bits from a CSPRNG: there is no guessing attack to slow down, and putting
// Argon2 on this path would add tens of milliseconds to every secret retrieval
// while buying nothing. What matters is that the database never holds the
// credential itself, and it does not.
func Hash(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Parse splits a presented credential into its kind and public handle without
// validating it against anything. It is used to find the candidate row.
func Parse(presented string) (Kind, string, error) {
	parts := strings.Split(presented, "_")
	if len(parts) != 3 {
		return "", "", ErrMalformed
	}
	kind, publicID, secret := Kind(parts[0]), parts[1], parts[2]

	switch kind {
	case KindMachine, KindRuntime, KindSession, KindInvite:
	default:
		return "", "", ErrMalformed
	}
	if len(publicID) != encoding.EncodedLen(publicIDBytes) {
		return "", "", ErrMalformed
	}
	if len(secret) != encoding.EncodedLen(secretBytes) {
		return "", "", ErrMalformed
	}
	return kind, publicID, nil
}

// Verify compares a presented credential against a stored verifier in constant
// time, so that comparison duration cannot be used to recover the stored value
// byte by byte.
func Verify(presented string, storedHash []byte) bool {
	candidate := Hash(presented)
	return subtle.ConstantTimeCompare(candidate, storedHash) == 1
}

// Display renders a credential for a listing: enough to recognize, never enough
// to use. This is the only representation of a credential that may be sent to a
// browser after the single moment of creation.
func Display(kind Kind, publicID string) string {
	return fmt.Sprintf("%s_%s_%s", kind, publicID, strings.Repeat("•", 8))
}
