package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

// LocalKeyringProvider holds key encryption keys in the process's own memory,
// loaded at startup from operator-supplied configuration.
//
// This provider exists for local development and for self-hosted deployments
// that inject key material through their own mechanism. It offers no protection
// against an attacker who can read the process's memory or environment, and it
// performs no key access logging. Production deployments should use a KMS or
// HSM-backed provider; see docs/security/key-management.md.
//
// The keyring is never read from the database and never from the source tree.
// It is supplied out of band, so that a database backup — which contains every
// ciphertext — is not by itself sufficient to recover any plaintext.
type LocalKeyringProvider struct {
	keys        map[string][]byte // key ID -> 32-byte KEK
	activeKeyID string
}

const (
	// localKeyPrefix namespaces key IDs by provider so that ciphertext written
	// under a development keyring is not silently attempted against a
	// production KMS after a misconfigured restore.
	localKeyPrefix = "local:"

	// kekSize is 32 bytes, selecting AES-256 for key wrapping.
	kekSize = 32
)

// NewLocalKeyringProvider builds a provider from a map of key version to raw
// 32-byte key, and the version new material should be written under.
func NewLocalKeyringProvider(keys map[string][]byte, activeVersion string) (*LocalKeyringProvider, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("crypto: keyring is empty")
	}

	loaded := make(map[string][]byte, len(keys))
	for version, key := range keys {
		if version == "" {
			return nil, fmt.Errorf("crypto: keyring contains an unnamed key version")
		}
		if len(key) != kekSize {
			// The length is safe to report; the key itself obviously is not.
			return nil, fmt.Errorf("crypto: key %q must be %d bytes, got %d", version, kekSize, len(key))
		}
		loaded[localKeyPrefix+version] = key
	}

	activeID := localKeyPrefix + activeVersion
	if _, ok := loaded[activeID]; !ok {
		return nil, fmt.Errorf("crypto: active key version %q is not present in the keyring", activeVersion)
	}

	return &LocalKeyringProvider{keys: loaded, activeKeyID: activeID}, nil
}

// ParseKeyring reads the keyring encoding used by configuration:
//
//	v1:<base64-32-bytes>,v2:<base64-32-bytes>
//
// Standard or URL-safe base64 is accepted, with or without padding.
func ParseKeyring(encoded string) (map[string][]byte, error) {
	keys := map[string][]byte{}
	for _, entry := range strings.Split(encoded, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		version, material, ok := strings.Cut(entry, ":")
		if !ok {
			return nil, fmt.Errorf("crypto: keyring entry must be formatted as version:base64key")
		}
		version = strings.TrimSpace(version)
		raw, err := decodeBase64(strings.TrimSpace(material))
		if err != nil {
			return nil, fmt.Errorf("crypto: keyring entry %q is not valid base64", version)
		}
		if _, exists := keys[version]; exists {
			return nil, fmt.Errorf("crypto: keyring contains duplicate version %q", version)
		}
		keys[version] = raw
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("crypto: keyring is empty")
	}
	return keys, nil
}

func decodeBase64(s string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		if raw, err := enc.DecodeString(s); err == nil {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}

// GenerateKEK returns a fresh 32-byte key encryption key for operators
// bootstrapping a deployment.
func GenerateKEK() ([]byte, error) {
	key := make([]byte, kekSize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("crypto: generating key: %w", err)
	}
	return key, nil
}

// ActiveKeyID implements KeyProvider.
func (p *LocalKeyringProvider) ActiveKeyID(context.Context) (string, error) {
	return p.activeKeyID, nil
}

// Wrap implements KeyProvider using AES-256-GCM, with the encryption context
// bound as additional authenticated data.
func (p *LocalKeyringProvider) Wrap(_ context.Context, keyID string, dataKey []byte, encCtx map[string]string) ([]byte, error) {
	aead, err := p.aead(keyID)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generating nonce: %w", err)
	}

	// The nonce is prepended to the wrapped key so the pair travels together;
	// a nonce is not secret, only single-use.
	sealed := aead.Seal(nil, nonce, dataKey, canonicalContext(encCtx))
	return append(nonce, sealed...), nil
}

// Unwrap implements KeyProvider.
func (p *LocalKeyringProvider) Unwrap(_ context.Context, keyID string, wrapped []byte, encCtx map[string]string) ([]byte, error) {
	aead, err := p.aead(keyID)
	if err != nil {
		return nil, err
	}
	if len(wrapped) < aead.NonceSize() {
		return nil, fmt.Errorf("crypto: wrapped key is malformed")
	}

	nonce, sealed := wrapped[:aead.NonceSize()], wrapped[aead.NonceSize():]
	dataKey, err := aead.Open(nil, nonce, sealed, canonicalContext(encCtx))
	if err != nil {
		// Authentication failed. This is either tampering or a mismatched
		// encryption context, and the two are not distinguished on purpose:
		// telling a caller which one occurred would let them probe the binding.
		return nil, fmt.Errorf("crypto: unwrapping data key: authentication failed")
	}
	return dataKey, nil
}

// Describe implements KeyProvider. It reports key versions, never key material.
func (p *LocalKeyringProvider) Describe() string {
	versions := make([]string, 0, len(p.keys))
	for id := range p.keys {
		versions = append(versions, strings.TrimPrefix(id, localKeyPrefix))
	}
	sort.Strings(versions)
	return fmt.Sprintf("local keyring (versions: %s, active: %s)",
		strings.Join(versions, ", "), strings.TrimPrefix(p.activeKeyID, localKeyPrefix))
}

func (p *LocalKeyringProvider) aead(keyID string) (cipher.AEAD, error) {
	key, ok := p.keys[keyID]
	if !ok {
		// Ciphertext references a key this deployment does not hold. Callers
		// translate this into a generic failure; see ErrKeyUnavailable.
		return nil, fmt.Errorf("%w: %q", ErrKeyUnavailable, keyID)
	}
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
