package credential

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// RecoveryCodeCount is how many codes are issued in a set.
const RecoveryCodeCount = 10

const (
	// recoveryCodeLength is the number of characters in one code, before
	// grouping. 15 characters of a 32-symbol alphabet is 75 bits, which is far
	// past brute force while still being something a person can copy off paper
	// without losing their place.
	recoveryCodeLength = 15

	// recoveryGroupSize breaks the code into runs of five, for the same reason
	// a card number is grouped: a human transcribing it needs somewhere to
	// rest.
	recoveryGroupSize = 5

	// recoveryAlphabet is Crockford's base32: no I, L, O or U. The first three
	// are excluded because they are indistinguishable from 1 and 0 in most
	// typefaces, and U because removing it keeps the alphabet from spelling
	// anything unfortunate.
	recoveryAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// NewRecoveryCodes returns a fresh set of codes in display form.
//
// Display form is what the person sees once and writes down. Everything that
// stores or compares a code uses NormalizeRecoveryCode first, so that a code
// typed back in lower case, with the dashes left out, or with spaces where the
// dashes were, is the same code.
func NewRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, RecoveryCodeCount)
	for i := 0; i < RecoveryCodeCount; i++ {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func newRecoveryCode() (string, error) {
	raw := make([]byte, recoveryCodeLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("credential: reading random bytes: %w", err)
	}

	var b strings.Builder
	for i, v := range raw {
		if i > 0 && i%recoveryGroupSize == 0 {
			b.WriteByte('-')
		}
		// Modulo bias here is negligible: 256 divided by a 32-symbol alphabet
		// is exact, so every symbol is equally likely.
		b.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
	}
	return b.String(), nil
}

// NormalizeRecoveryCode puts a code into the one form that is ever hashed.
//
// People retype these from paper or a password manager, so the separators and
// the case they arrive in say nothing about whether the code is right.
func NormalizeRecoveryCode(presented string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(presented)) {
		if strings.ContainsRune(recoveryAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HashRecoveryCode returns the stored form of a code.
func HashRecoveryCode(presented string) []byte {
	return Hash(NormalizeRecoveryCode(presented))
}
