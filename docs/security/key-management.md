# Key management

The key encryption key is the single point on which every stored secret depends.
Lose it and the ciphertext is unrecoverable. Leak it alongside a database dump
and every credential in the organization is exposed. It deserves more care than
anything else in the deployment.

## The hierarchy

```
Key encryption key (KEK)      operator-supplied, or held by a KMS
        │  wraps
        ▼
Data encryption key (DEK)     generated per secret version, used once
        │  encrypts
        ▼
Ciphertext                    stored in secret_material
```

A fresh data key per version means a recovered data key exposes one version of
one secret rather than a corpus, and it removes nonce-reuse risk on values
entirely, since each data key performs exactly one encryption.

The KEK never enters the database and never enters the source tree. That is what
makes a database backup insufficient on its own to recover any plaintext.

## Local development

```bash
go run ./cmd/warder-api keygen
```

This prints one line to stdout, in the keyring format, plus guidance to stderr:

```
v1:Base64EncodedThirtyTwoByteKeyGoesHere...
```

Put it in `.env` as `WARDER_KEYRING`. It is git-ignored. Nothing else needs to
know it exists.

The local provider holds keys in process memory. It offers no protection against
anyone who can read that memory or the process environment, and it logs no key
access. It is for development and for self-hosted deployments that inject key
material through their own mechanism: not for a deployment where the threat
model includes the host.

## Production

Use a KMS. `crypto.KeyProvider` exists so that this is a wiring change:

```go
type KeyProvider interface {
    ActiveKeyID(ctx context.Context) (string, error)
    Wrap(ctx context.Context, keyID string, dataKey []byte, encCtx map[string]string) ([]byte, error)
    Unwrap(ctx context.Context, keyID string, wrapped []byte, encCtx map[string]string) ([]byte, error)
    Describe() string
}
```

The shape maps directly onto each service:

| Provider | Mechanism | Encryption context |
|---|---|---|
| AWS KMS | `Encrypt`/`Decrypt` on a customer managed key | `EncryptionContext`, passed through verbatim |
| GCP Cloud KMS | `Encrypt`/`Decrypt` on a crypto key | `additionalAuthenticatedData`, canonical serialization |
| Azure Key Vault / Managed HSM | `wrapKey`/`unwrapKey` | No native context; carried by a local AEAD layer over the unwrapped key |

Whichever is chosen, three things must hold:

1. **The encryption context must be passed through.** It is what binds ciphertext
   to its location in the secret tree. A provider that silently drops it removes
   a real defence: see the threat model's note on relocated ciphertext.
2. **The key must not be exportable.** The point of a KMS is that a compromise of
   the application does not yield the key.
3. **Key access must be logged by the KMS.** Warder's audit trail records secret
   use; the KMS's records key use. During an incident you want both.

Grant the application's principal `Encrypt` and `Decrypt` only. It has no need to
create, schedule deletion of, or change the policy on a key.

## Rotation

The schema records the key version that wrapped each row's data key, so rotation
does not require rewriting the database.

```
1. Generate a new key.
2. Add it alongside the existing one:
     WARDER_KEYRING=v1:<old>,v2:<new>
3. Point new writes at it:
     WARDER_ACTIVE_KEY_VERSION=v2
4. Restart.
```

From that moment new versions are written under `v2` while everything under `v1`
keeps decrypting normally. This is verified in
`TestDecryptSupportsOlderKeyVersions`.

**Do not remove `v1` from the keyring** until nothing references it:

```sql
SELECT encryption_key_id, count(*)
FROM secret_versions
GROUP BY encryption_key_id;
```

Removing a key that rows still reference makes those rows permanently
unreadable. The failure is loud, `ErrKeyUnavailable` internally, a generic
"unavailable" to callers, but it is not recoverable without the key.

### Never reuse a version name

A version name identifies one specific key, permanently. Putting a *different*
key under a name that has already been used is the worst version of this
mistake, and it is worth understanding why it is worse than simply deleting a
key.

Each row records the version that sealed it. When that version is absent, the
provider reports `ErrKeyUnavailable`: "this deployment does not hold the key
that wrote this row": which is unambiguous and points straight at the
keyring. When the version is *present but holds different key material*, the
lookup succeeds and the AEAD authentication fails instead. That failure is
indistinguishable from tampering, because it is meant to be: the whole point of
authenticated encryption is that a wrong key and a forged ciphertext produce
the same answer. So a name collision surfaces as a suspected attack rather than
as a configuration error, and sends whoever is on call in entirely the wrong
direction.

`warder-api init` always emits `v1`, which makes it a first-run command only.
It warns about this when you run it. To add a key to a deployment that already
holds secrets, use `keygen` and append the result under a new name.

### Re-encryption

Not implemented, and the schema is ready for it. The procedure would be: read a
version's material, decrypt under its recorded key, re-encrypt under the active
key with the same encryption context, and replace the material row in one
transaction. Because the encryption context includes the version number and does
not change, the re-encrypted value remains valid in place.

This is deliberately deferred. Rotation of the *active* key is the operation that
matters after a suspected exposure, and it takes effect immediately for all new
writes; re-encrypting history is a background concern.

## If a key is exposed

Rotating the KEK does not help on its own, the exposed key still decrypts every
row written under it, and an attacker with a database copy already has those
rows. The order that matters is:

1. **Rotate the underlying credentials at their providers.** New database
   passwords, new API keys. This is what actually revokes the attacker's access.
2. Store the new values in Warder, which writes them under the current key.
3. Rotate the KEK and re-encrypt, so old ciphertext stops being useful.
4. Revoke tokens and sessions.
5. Read the audit trail for `SECRET_USED` and `SECRET_REVEALED` to scope the
   exposure.

Step 1 is the one people skip, and it is the only one that stops the attacker.

## Backup

Key material and database backups must not be stored together. A backup system
that captures both in one place has, in effect, stored the secrets in plaintext:
whoever holds that backup holds everything.

- Database backups: encrypted at rest, standard retention.
- Key material: in the KMS, or in a separate secret manager with its own access
  control and its own audit trail.
- Test restores must exercise both halves. A restore rehearsal that uses the
  production keyring has not proven the keyring is recoverable.
