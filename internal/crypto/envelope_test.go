package crypto

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func testService(t *testing.T) (*EnvelopeEncryptionService, *LocalKeyringProvider) {
	t.Helper()

	k1, err := GenerateKEK()
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	k2, err := GenerateKEK()
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	provider, err := NewLocalKeyringProvider(map[string][]byte{"v1": k1, "v2": k2}, "v2")
	if err != nil {
		t.Fatalf("building provider: %v", err)
	}
	return NewEnvelopeEncryptionService(provider), provider
}

func testContext() EncryptionContext {
	return EncryptionContext{
		OrganizationID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		ProjectID:      uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		EnvironmentID:  uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		SecretID:       uuid.MustParse("44444444-4444-4444-8444-444444444444"),
		Version:        1,
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	ec := testContext()

	plaintext := []byte("postgres://broker:not-a-real-password@db.internal:5432/payments")

	env, err := svc.Encrypt(ctx, plaintext, ec)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if bytes.Contains(env.Ciphertext, plaintext) {
		t.Fatal("ciphertext contains the plaintext")
	}
	if env.KeyID != "local:v2" {
		t.Fatalf("expected new material under the active key, got %q", env.KeyID)
	}

	got, err := svc.Decrypt(ctx, env, ec)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("round trip did not preserve the value")
	}
}

// Each version must get its own data key, so that one recovered data key
// exposes exactly one version of one secret.
func TestEncryptUsesFreshDataKeyPerCall(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	ec := testContext()

	a, err := svc.Encrypt(ctx, []byte("same value"), ec)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	b, err := svc.Encrypt(ctx, []byte("same value"), ec)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if bytes.Equal(a.WrappedDataKey, b.WrappedDataKey) {
		t.Fatal("the same wrapped data key was reused across encryptions")
	}
	if bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Fatal("identical plaintext produced identical ciphertext")
	}
	if bytes.Equal(a.Nonce, b.Nonce) {
		t.Fatal("nonce was reused")
	}
}

// The central integrity property: ciphertext only opens in the location it was
// written to. An attacker with database write access cannot relocate a
// production ciphertext into a secret they are authorized to use.
func TestDecryptRejectsRelocatedCiphertext(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	production := testContext()
	env, err := svc.Encrypt(ctx, []byte("sk_live_definitely_not_real"), production)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	relocations := map[string]func(EncryptionContext) EncryptionContext{
		"different organization": func(c EncryptionContext) EncryptionContext {
			c.OrganizationID = uuid.New()
			return c
		},
		"different project": func(c EncryptionContext) EncryptionContext {
			c.ProjectID = uuid.New()
			return c
		},
		"different environment": func(c EncryptionContext) EncryptionContext {
			c.EnvironmentID = uuid.New()
			return c
		},
		"different secret": func(c EncryptionContext) EncryptionContext {
			c.SecretID = uuid.New()
			return c
		},
		"different version": func(c EncryptionContext) EncryptionContext {
			c.Version = c.Version + 1
			return c
		},
	}

	for name, relocate := range relocations {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Decrypt(ctx, env, relocate(production))
			if !errors.Is(err, ErrDecryptionFailed) {
				t.Fatalf("relocated ciphertext decrypted, or failed with the wrong error: %v", err)
			}
		})
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	ec := testContext()

	env, err := svc.Encrypt(ctx, []byte("value under protection"), ec)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	tamper := map[string]func(*EncryptedSecret){
		"flip a ciphertext bit":  func(e *EncryptedSecret) { e.Ciphertext[0] ^= 0x01 },
		"flip a tag bit":         func(e *EncryptedSecret) { e.Ciphertext[len(e.Ciphertext)-1] ^= 0x01 },
		"flip a nonce bit":       func(e *EncryptedSecret) { e.Nonce[0] ^= 0x01 },
		"flip a wrapped key bit": func(e *EncryptedSecret) { e.WrappedDataKey[0] ^= 0x01 },
		"truncate ciphertext":    func(e *EncryptedSecret) { e.Ciphertext = e.Ciphertext[:len(e.Ciphertext)-1] },
	}

	for name, mutate := range tamper {
		t.Run(name, func(t *testing.T) {
			corrupted := *env
			corrupted.Ciphertext = bytes.Clone(env.Ciphertext)
			corrupted.Nonce = bytes.Clone(env.Nonce)
			corrupted.WrappedDataKey = bytes.Clone(env.WrappedDataKey)
			mutate(&corrupted)

			if _, err := svc.Decrypt(ctx, &corrupted, ec); !errors.Is(err, ErrDecryptionFailed) {
				t.Fatalf("tampered envelope was accepted, or failed with the wrong error: %v", err)
			}
		})
	}
}

// Ciphertext written under an older key must keep decrypting after the active
// key moves on; otherwise key rotation would require a synchronous rewrite of
// every row.
func TestDecryptSupportsOlderKeyVersions(t *testing.T) {
	k1, _ := GenerateKEK()
	k2, _ := GenerateKEK()
	ctx := context.Background()
	ec := testContext()

	oldProvider, err := NewLocalKeyringProvider(map[string][]byte{"v1": k1}, "v1")
	if err != nil {
		t.Fatalf("building provider: %v", err)
	}
	env, err := NewEnvelopeEncryptionService(oldProvider).Encrypt(ctx, []byte("written under v1"), ec)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if env.KeyID != "local:v1" {
		t.Fatalf("expected local:v1, got %q", env.KeyID)
	}

	// The keyring now holds both keys and writes new material under v2.
	rotated, err := NewLocalKeyringProvider(map[string][]byte{"v1": k1, "v2": k2}, "v2")
	if err != nil {
		t.Fatalf("building provider: %v", err)
	}
	svc := NewEnvelopeEncryptionService(rotated)

	got, err := svc.Decrypt(ctx, env, ec)
	if err != nil {
		t.Fatalf("decrypting v1 material after rotation: %v", err)
	}
	if string(got) != "written under v1" {
		t.Fatal("value was not preserved across key rotation")
	}

	// Re-encrypting produces material under the new active key, which is the
	// mechanism a background re-encryption job would use.
	reencrypted, err := svc.Encrypt(ctx, got, ec)
	if err != nil {
		t.Fatalf("re-encrypt: %v", err)
	}
	if reencrypted.KeyID != "local:v2" {
		t.Fatalf("re-encryption did not adopt the active key, got %q", reencrypted.KeyID)
	}
}

// A database restored alongside the wrong keyring must fail closed, and must
// not describe what is missing in terms a caller could learn from.
func TestDecryptFailsClosedWhenKeyIsAbsent(t *testing.T) {
	k1, _ := GenerateKEK()
	k9, _ := GenerateKEK()
	ctx := context.Background()
	ec := testContext()

	original, _ := NewLocalKeyringProvider(map[string][]byte{"v1": k1}, "v1")
	env, err := NewEnvelopeEncryptionService(original).Encrypt(ctx, []byte("orphaned"), ec)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	unrelated, _ := NewLocalKeyringProvider(map[string][]byte{"v9": k9}, "v9")
	_, err = NewEnvelopeEncryptionService(unrelated).Decrypt(ctx, env, ec)

	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected a decryption failure, got %v", err)
	}
	if !errors.Is(err, ErrKeyUnavailable) {
		t.Fatalf("operators need to distinguish a missing key internally, got %v", err)
	}
}

// No error on the decryption path may quote the material it was handling.
func TestDecryptionErrorsCarryNoPlaintext(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	ec := testContext()

	const plaintext = "sk_live_leak_canary_value"
	env, err := svc.Encrypt(ctx, []byte(plaintext), ec)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	wrong := ec
	wrong.SecretID = uuid.New()
	_, err = svc.Decrypt(ctx, env, wrong)
	if err == nil {
		t.Fatal("expected failure")
	}
	if strings.Contains(err.Error(), plaintext) {
		t.Fatal("decryption error leaked the plaintext")
	}
	if strings.Contains(err.Error(), ec.SecretID.String()) {
		t.Fatal("decryption error leaked the encryption context")
	}
}

// An all-zero context would bind ciphertext to nothing, so it is refused.
func TestEncryptRequiresCompleteContext(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	incomplete := []EncryptionContext{
		{},
		{OrganizationID: uuid.New()},
		{OrganizationID: uuid.New(), ProjectID: uuid.New()},
		{OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New()},
		{OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(), SecretID: uuid.New()},
	}

	for _, ec := range incomplete {
		if _, err := svc.Encrypt(ctx, []byte("x"), ec); err == nil {
			t.Fatalf("encryption accepted an incomplete context: %+v", ec)
		}
	}
}

func TestKeyringRejectsBadMaterial(t *testing.T) {
	short := make([]byte, 16)

	if _, err := NewLocalKeyringProvider(map[string][]byte{"v1": short}, "v1"); err == nil {
		t.Fatal("accepted a 16-byte key encryption key")
	}
	if _, err := NewLocalKeyringProvider(map[string][]byte{}, "v1"); err == nil {
		t.Fatal("accepted an empty keyring")
	}

	full, _ := GenerateKEK()
	if _, err := NewLocalKeyringProvider(map[string][]byte{"v1": full}, "v7"); err == nil {
		t.Fatal("accepted an active version absent from the keyring")
	}
}

func TestParseKeyring(t *testing.T) {
	k1, _ := GenerateKEK()
	k2, _ := GenerateKEK()

	encoded := "v1:" + base64Std(k1) + ",v2:" + base64Std(k2)
	keys, err := ParseKeyring(encoded)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(keys) != 2 || !bytes.Equal(keys["v1"], k1) || !bytes.Equal(keys["v2"], k2) {
		t.Fatal("keyring did not round trip")
	}

	if _, err := ParseKeyring("v1:" + base64Std(k1) + ",v1:" + base64Std(k2)); err == nil {
		t.Fatal("accepted duplicate key versions")
	}
	if _, err := ParseKeyring("no-separator"); err == nil {
		t.Fatal("accepted a malformed entry")
	}
}

// Describe feeds startup logs, so it must never carry key material.
func TestDescribeRevealsNoKeyMaterial(t *testing.T) {
	k1, _ := GenerateKEK()
	provider, _ := NewLocalKeyringProvider(map[string][]byte{"v1": k1}, "v1")

	description := provider.Describe()
	if strings.Contains(description, base64Std(k1)) || bytes.Contains([]byte(description), k1) {
		t.Fatal("provider description leaked key material")
	}
}

// The canonical encoding must not be ambiguous: no two distinct contexts may
// serialize to the same AAD, or the binding could be sidestepped by crafting
// names that collide.
func TestCanonicalContextIsUnambiguous(t *testing.T) {
	a := canonicalContext(map[string]string{"a": "b;c=d", "e": "f"})
	b := canonicalContext(map[string]string{"a": "b", "c": "d", "e": "f"})
	if bytes.Equal(a, b) {
		t.Fatal("distinct contexts produced identical additional authenticated data")
	}

	// Ordering of map keys must not affect the result.
	first := canonicalContext(map[string]string{"z": "1", "a": "2", "m": "3"})
	second := canonicalContext(map[string]string{"m": "3", "z": "1", "a": "2"})
	if !bytes.Equal(first, second) {
		t.Fatal("canonical encoding depends on map iteration order")
	}
}

func TestZeroize(t *testing.T) {
	buf := []byte("sensitive material")
	Zeroize(buf)
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("byte %d was not cleared", i)
		}
	}
	Zeroize(nil) // must not panic
}
