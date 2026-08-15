# Audit

Two rules hold everywhere in the audit system. Events cannot be altered or
deleted by the running application. And no event carries secret material — an
event records that `DATABASE_URL` was used, never what `DATABASE_URL` is.

## Append-only, enforced twice

```sql
CREATE TRIGGER audit_events_no_update
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION deny_audit_mutation();
```

```sql
REVOKE UPDATE, DELETE ON audit_events FROM warder_app;
```

The trigger and the privilege revocation are independent: the trigger holds even
if privileges are misconfigured, and the revocation holds even if the trigger is
dropped. An attacker who reaches the application's database role can still write
misleading events — nothing prevents that — but cannot erase the record of what
they did.

**Consequence worth knowing before you meet it:** the trigger also blocks the
cascade from deleting an organization. That is intended. Erasing the record of
what happened should require someone to decide to erase it, so it is a privileged
procedure (`deploy/sql/erase-organization.sql`, run as the table owner) rather
than a side effect of an ordinary `DELETE`.

## No values, structurally

The `Event` type has no field capable of holding a value. The only open-ended
field is `Metadata`, and everything passing through it is scrubbed on the way in:

- keys matching the sensitive-name list are replaced outright
- string values are scanned for credential shapes, including Warder's own token
  format and common vendor formats
- a `secretvalue.Value` is replaced with `[redacted]` before serialization

So an event records `secret_key: "DATABASE_URL"` and there is no path by which it
records the connection string.

## What is recorded

| Category | Events |
|---|---|
| Secrets | `SECRET_CREATED` `SECRET_ROTATED` `SECRET_ROLLED_BACK` `SECRET_REVOKED` `SECRET_EXPIRY_CHANGED` `SECRET_DELETED` |
| Use | `SECRET_USED` — one per key, per delivery, success or failure |
| Disclosure | `SECRET_REVEAL_REQUESTED` `SECRET_REVEALED` |
| Access | `ACCESS_GRANTED` `ACCESS_REVOKED` `ACCESS_DENIED` |
| Credentials | `TOKEN_CREATED` `TOKEN_REVOKED` `RUNTIME_AUTHENTICATED` |
| Identity | `IDENTITY_CREATED` `IDENTITY_DISABLED` `USER_INVITED` `USER_REMOVED` |
| Sessions | `LOGIN` `LOGIN_FAILED` `LOGOUT` |
| Failures | `DECRYPTION_FAILED` `RATE_LIMITED` |

Each carries the actor and their type, the credential used, the project,
environment, secret key, client address, user agent, outcome, and — for denials —
a stable `deny_code` alongside a human-readable reason.

Denials are recorded as deliberately as successes. A `SECRET_REVEAL_REQUESTED`
followed by a denied `SECRET_REVEALED` is often the more interesting pair.

## Transactional where it matters

State changes and their audit events commit together:

```go
store.InTx(ctx, db, func(tx pgx.Tx) error {
    version, err := secrets.CreateVersion(ctx, tx, ...)
    if err != nil { return err }
    return audit.RecordTx(ctx, tx, event)
})
```

A rotation that happened without a record, or a record of one that did not, would
each undermine the trail. Read-path events (`SECRET_USED`) are written outside
the transaction on a context detached from the request, so a client disconnecting
mid-request cannot prevent the record of a delivery that already happened.

## Questions it answers

**Who can use this secret, and who can see it?**
The access screen, from `GET /projects/{id}/access` — which reports capabilities
individually rather than as a role, because those are two different answers.

**Where is it being used, and when was it last?**
`last_used_at` on the secret, and `SECRET_USED` events filtered by secret.

```sql
SELECT actor_label, actor_type, max(occurred_at) AS last_used, count(*) AS uses
FROM audit_events
WHERE secret_id = $1 AND event_type = 'SECRET_USED' AND outcome = 'SUCCESS'
GROUP BY 1, 2 ORDER BY last_used DESC;
```

**Who has been granted plaintext visibility, and why?**

```sql
SELECT occurred_at, actor_label, reason,
       metadata->>'subject_name'  AS granted_to,
       metadata->>'self_granted'  AS self_granted
FROM audit_events
WHERE event_type = 'ACCESS_GRANTED'
  AND metadata->>'grants_plaintext_visibility' = 'true'
ORDER BY occurred_at DESC;
```

That flag exists so this is one filter rather than a scan of every grant event.

**What was denied, and why?**

```sql
SELECT occurred_at, actor_label, actor_type, secret_key,
       reason, metadata->>'deny_code' AS code
FROM audit_events
WHERE outcome = 'DENIED' AND occurred_at > now() - interval '24 hours'
ORDER BY occurred_at DESC;
```

## Gaps

- **Nothing watches these events.** A burst of `ACCESS_DENIED`, an unexpected
  `SECRET_REVEALED`, or a `DECRYPTION_FAILED` should page someone. Alerting is
  not built.
- **Retention is unbounded.** Trimming requires the privileged path.
- **Events live only in this database.** A point-in-time restore loses the events
  from the window it rolls back. If audit continuity matters, ship events to an
  external sink as they are written.
