package crypto

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// EncryptionContext identifies exactly where a piece of ciphertext belongs in
// the secret tree. It is bound into the ciphertext as AEAD additional
// authenticated data, and passed to the key provider as a key-management
// encryption context.
//
// This binding is a deliberate defense against an attacker who has gained write
// access to the database but not to the key material. Without it, such an
// attacker could copy the production DATABASE_URL ciphertext row into a
// development secret they are authorized to use, and have the broker decrypt it
// for them: a full compromise achieved without ever breaking encryption. With
// it, the ciphertext only decrypts in the exact location it was written to, so
// relocated ciphertext fails authentication and is reported as a decryption
// failure.
type EncryptionContext struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	SecretID       uuid.UUID
	Version        int
}

// Validate rejects a context that is missing any binding component. An
// all-zero context would bind ciphertext to nothing and silently remove the
// protection above, so it is treated as a programming error rather than
// tolerated.
func (c EncryptionContext) Validate() error {
	switch {
	case c.OrganizationID == uuid.Nil:
		return fmt.Errorf("encryption context: missing organization")
	case c.ProjectID == uuid.Nil:
		return fmt.Errorf("encryption context: missing project")
	case c.EnvironmentID == uuid.Nil:
		return fmt.Errorf("encryption context: missing environment")
	case c.SecretID == uuid.Nil:
		return fmt.Errorf("encryption context: missing secret")
	case c.Version <= 0:
		return fmt.Errorf("encryption context: missing version")
	}
	return nil
}

// Map renders the context in the key/value form used by cloud KMS encryption
// contexts (AWS KMS EncryptionContext, GCP additional authenticated data,
// Azure Key Vault). Keeping this shape means the local development provider and
// a future managed-HSM provider bind exactly the same facts.
func (c EncryptionContext) Map() map[string]string {
	return map[string]string{
		"org":     c.OrganizationID.String(),
		"project": c.ProjectID.String(),
		"env":     c.EnvironmentID.String(),
		"secret":  c.SecretID.String(),
		"version": fmt.Sprintf("%d", c.Version),
	}
}

// canonicalContext serializes a context map deterministically for use as AEAD
// additional authenticated data.
//
// The encoding is length-prefixed rather than merely delimited so that no
// combination of key or value contents can produce two different maps with the
// same serialization. Keys are sorted so the same map always yields the same
// bytes regardless of Go's map iteration order.
func canonicalContext(m map[string]string) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%d:%s=%d:%s;", len(k), k, len(m[k]), m[k])
	}
	return []byte(b.String())
}
