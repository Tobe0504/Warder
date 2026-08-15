# Disaster recovery

Recovering Warder means recovering two things that must be stored apart: the
database, and the key material. Neither is useful alone. That is the design
working as intended, and it is also the thing most likely to be got wrong during
a restore rehearsal.

## What must be backed up

| Component | Contains | Stored |
|---|---|---|
| PostgreSQL | Ciphertext, metadata, grants, audit | Encrypted at rest, standard retention |
| Key encryption keys | The ability to read any of it | KMS, or a separate secret manager |
| Configuration | Service credential, connection strings | Deployment's own secret store |

**The first two must never share a location.** A backup archive containing both
is equivalent to a plaintext copy of every credential in the system.

## Restore procedure

```bash
# 1. Restore the database.
pg_restore --clean --if-exists -d "$WARDER_MIGRATION_DATABASE_URL" backup.dump

# 2. Confirm the keyring matches. Every key version referenced by a row must be
#    present, or those rows are unreadable.
psql "$WARDER_DATABASE_URL" -c \
  "SELECT encryption_key_id, count(*) FROM secret_versions GROUP BY 1;"

# 3. Apply any migrations the backup predates.
./warder-api migrate

# 4. Start, and verify.
./warder-api serve
```

Verification means retrieving one known secret through the runtime API and
confirming the value is correct. A service that starts successfully has proven
nothing about whether it can decrypt.

## What happens when keys are unavailable

The system fails closed and stays useful for everything that does not require
decryption.

| Symptom | Behaviour |
|---|---|
| Key version missing from the keyring | Affected secrets report `secret_unavailable`; `ErrKeyUnavailable` in the logs |
| KMS unreachable | Decryption fails; metadata, audit, and access management continue to work |
| Wrong keyring entirely | Every decryption fails authentication; nothing is silently mis-decrypted |

The last row is worth dwelling on: because the AEAD authenticates, a wrong key
produces a failure rather than plausible-looking garbage. There is no state in
which Warder hands a runtime a value that is not the value that was stored.

Metadata, grants, tokens, and the audit trail remain fully readable throughout,
which means an incident responder can still answer "who had access to what" while
decryption is broken.

## Point-in-time restore

Restoring the database to an earlier moment un-does more than it appears to:

- **Secrets rotated after the restore point revert to their previous values.**
  The credential at the provider has not reverted, so applications will
  authenticate with a stale value and fail. Re-enter the current values.
- **Access revoked after the restore point comes back.** Anyone offboarded in
  that window regains access. Re-apply those revocations, and check
  `ACCESS_REVOKED` and `USER_REMOVED` events from the lost window against the
  restored state.
- **Tokens revoked in that window become live again.** Same treatment.
- **Audit events from the lost window are gone.** They cannot be reconstructed.
  If audit continuity matters, ship events to an external sink as they are
  written rather than relying on database backups.

A restore checklist should have those four items on it explicitly, because each
is a security regression that looks like a successful recovery.

## Recovery targets

The MVP defines no RTO or RPO, and one should be set before real use. What the
architecture supports:

- The API is stateless. Recovery time is database restore time.
- Warm standby is straightforward, as long as the standby has key access, which
  is exactly the thing that gets forgotten.
- Runtime sessions live minutes and simply re-authenticate.
- Long-lived machine tokens survive a restore, provided the restore point
  predates their creation. Tokens minted after the restore point will not exist
  and must be reissued.

## Rehearsal

A restore rehearsal that has not tested key recovery has tested the easy half.
The rehearsal should:

1. Restore into an isolated environment.
2. Retrieve the keyring **through the same path a real incident would use**: not
   from a developer's `.env`, not from a running production process.
3. Start the service and successfully retrieve a known secret.
4. Confirm the audit trail is intact and queryable.

Step 2 is the whole exercise. Everything else is a database restore.
