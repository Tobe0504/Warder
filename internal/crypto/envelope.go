package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// ErrDecryptionFailed is the single error every decryption failure collapses
// into: a wrong key, a tampered ciphertext, a relocated row, and a truncated
// blob are indistinguishable to the caller.
//
// This is deliberate. Distinguishing them would turn the decryption path into
// an oracle that tells an attacker which part of their forgery was wrong.
var ErrDecryptionFailed = errors.New("secret material could not be decrypted")

// EnvelopeScheme identifies the envelope format so that the layout can change
// without ambiguity about how to read existing rows.
const EnvelopeScheme = 1

// AlgorithmAESGCM names the AEAD used for secret values.
const AlgorithmAESGCM = "AES-256-GCM"

const (
	dekSize   = 32 // AES-256 data encryption key
	nonceSize = 12 // 96-bit nonce, the size GCM is specified for
)

// EncryptedSecret is the stored envelope for one secret version. Every field is
// safe to persist; none of them individually or together yields plaintext
// without the key encryption key, which lives outside the database.
type EncryptedSecret struct {
	// Scheme is the envelope format version.
	Scheme int

	// Algorithm names the AEAD used for the value itself.
	Algorithm string

	// KeyID is the key encryption key version that wrapped DataKey. Recording
	// it per row is what allows key rotation to proceed incrementally.
	KeyID string

	// WrappedDataKey is the per-version data key, encrypted by the KeyProvider.
	WrappedDataKey []byte

	// Nonce is the AEAD nonce for Ciphertext.
	Nonce []byte

	// Ciphertext is the encrypted value with the GCM authentication tag
	// appended, as produced by Go's cipher.AEAD.
	Ciphertext []byte
}

// SecretEncryptionService is the only place in the system where plaintext
// secret values meet cryptography. Handlers, repositories, and services call
// this interface; none of them construct ciphers themselves.
type SecretEncryptionService interface {
	// Encrypt seals plaintext under a fresh data key bound to ec.
	Encrypt(ctx context.Context, plaintext []byte, ec EncryptionContext) (*EncryptedSecret, error)

	// Decrypt opens an envelope. It fails unless ec matches the context used at
	// encryption time exactly.
	Decrypt(ctx context.Context, enc *EncryptedSecret, ec EncryptionContext) ([]byte, error)

	// ActiveKeyID reports the key new material is written under, for display in
	// operational tooling.
	ActiveKeyID(ctx context.Context) (string, error)
}

// EnvelopeEncryptionService implements SecretEncryptionService using envelope
// encryption: a unique data key per secret version encrypts the value, and the
// KeyProvider wraps that data key under a root key the broker never handles.
//
// One data key per version means a data key is used for exactly one encryption,
// which removes nonce-reuse risk on the value ciphertext entirely, and means
// compromise of a single unwrapped data key exposes one version of one secret
// rather than a corpus.
type EnvelopeEncryptionService struct {
	keys KeyProvider
}

// NewEnvelopeEncryptionService constructs the service.
func NewEnvelopeEncryptionService(keys KeyProvider) *EnvelopeEncryptionService {
	return &EnvelopeEncryptionService{keys: keys}
}

var _ SecretEncryptionService = (*EnvelopeEncryptionService)(nil)

// ActiveKeyID implements SecretEncryptionService.
func (s *EnvelopeEncryptionService) ActiveKeyID(ctx context.Context) (string, error) {
	return s.keys.ActiveKeyID(ctx)
}

// Encrypt implements SecretEncryptionService.
func (s *EnvelopeEncryptionService) Encrypt(ctx context.Context, plaintext []byte, ec EncryptionContext) (*EncryptedSecret, error) {
	if err := ec.Validate(); err != nil {
		return nil, err
	}

	keyID, err := s.keys.ActiveKeyID(ctx)
	if err != nil {
		return nil, fmt.Errorf("crypto: resolving active key: %w", err)
	}

	dataKey := make([]byte, dekSize)
	if _, err := rand.Read(dataKey); err != nil {
		return nil, fmt.Errorf("crypto: generating data key: %w", err)
	}
	// The data key is wiped as soon as this function returns; it exists only
	// for the duration of this one encryption.
	defer Zeroize(dataKey)

	aead, err := newAEAD(dataKey)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generating nonce: %w", err)
	}

	encCtx := ec.Map()
	ciphertext := aead.Seal(nil, nonce, plaintext, canonicalContext(encCtx))

	wrapped, err := s.keys.Wrap(ctx, keyID, dataKey, encCtx)
	if err != nil {
		return nil, fmt.Errorf("crypto: wrapping data key: %w", err)
	}

	return &EncryptedSecret{
		Scheme:         EnvelopeScheme,
		Algorithm:      AlgorithmAESGCM,
		KeyID:          keyID,
		WrappedDataKey: wrapped,
		Nonce:          nonce,
		Ciphertext:     ciphertext,
	}, nil
}

// Decrypt implements SecretEncryptionService.
//
// Every failure below returns ErrDecryptionFailed with no detail. The internal
// cause is available to the caller for logging via errors.Join, but the
// sentinel is what surfaces to API responses.
func (s *EnvelopeEncryptionService) Decrypt(ctx context.Context, enc *EncryptedSecret, ec EncryptionContext) ([]byte, error) {
	if enc == nil {
		return nil, ErrDecryptionFailed
	}
	if err := ec.Validate(); err != nil {
		return nil, errors.Join(ErrDecryptionFailed, err)
	}
	if enc.Scheme != EnvelopeScheme {
		return nil, errors.Join(ErrDecryptionFailed,
			fmt.Errorf("unsupported envelope scheme %d", enc.Scheme))
	}
	if enc.Algorithm != AlgorithmAESGCM {
		return nil, errors.Join(ErrDecryptionFailed,
			fmt.Errorf("unsupported algorithm %q", enc.Algorithm))
	}

	encCtx := ec.Map()

	dataKey, err := s.keys.Unwrap(ctx, enc.KeyID, enc.WrappedDataKey, encCtx)
	if err != nil {
		return nil, errors.Join(ErrDecryptionFailed, err)
	}
	defer Zeroize(dataKey)

	if len(dataKey) != dekSize {
		return nil, errors.Join(ErrDecryptionFailed, errors.New("unwrapped data key has wrong length"))
	}

	aead, err := newAEAD(dataKey)
	if err != nil {
		return nil, errors.Join(ErrDecryptionFailed, err)
	}
	if len(enc.Nonce) != aead.NonceSize() {
		return nil, errors.Join(ErrDecryptionFailed, errors.New("nonce has wrong length"))
	}

	plaintext, err := aead.Open(nil, enc.Nonce, enc.Ciphertext, canonicalContext(encCtx))
	if err != nil {
		// Authentication failure: tampered ciphertext, or ciphertext presented
		// under a context other than the one it was written with.
		return nil, errors.Join(ErrDecryptionFailed, errors.New("authentication failed"))
	}
	return plaintext, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: initializing cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: initializing AEAD: %w", err)
	}
	return aead, nil
}
