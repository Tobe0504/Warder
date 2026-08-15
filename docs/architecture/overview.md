# Architecture

## The idea

Traditional secret management moves a credential to whoever needs the software to
run:

```
Organization → secret manager → developer → .env → application
```

Every arrow is a copy, and every copy is permanent. Once a developer, a
contractor, or an AI agent has read a value, the only way to un-share it is to
rotate the credential everywhere it is used.

Warder changes what flows:

```
Organization → access policy → runtime identity → application
                                     ↑
                             secret injected at runtime
```

The credential goes to the process. The person gets the ability to start that
process. Those are different grants, and separating them is the entire product.

## Repository layout

Go convention rather than `/apps` and `/packages`, because a Go module wants
`cmd/` and `internal/` and fighting that produces import paths nobody enjoys
reading. The separation the brief asked for is preserved; only the directory
names differ.

```
cmd/
  warder-api/         the core API binary (serve, migrate, keygen)
  ward/               the developer CLI

internal/
  domain/             capabilities, identities, entities, the vocabulary
  crypto/             envelope encryption and the key provider interface
  authz/              the policy engine, and nothing else
  identity/           credential → principal; the workload-identity seam
  secrets/            the service that orders authorize → decrypt → audit
  store/              every SQL statement in the system
  httpapi/            HTTP handlers, middleware, two surfaces
  audit/              event recording, with scrubbing on the way in
  credential/         token minting, verification, password hashing
  secretvalue/        the type plaintext travels in
  logging/            redaction
  ratelimit/          throttling
  config/             validated startup configuration
  cli/                CLI implementation
  apitest/            integration and security tests against a real database

migrations/           embedded SQL schema history
web/                  Next.js dashboard and BFF
deploy/               docker compose, least-privilege role setup
docs/                 this
```

## The two surfaces

The core API serves two HTTP listeners on different addresses. This is the
architecture's load-bearing decision.

```
Browser ──HTTPS──> Next.js BFF ──service credential──> Admin API   :8080
                                                          │
Runtime ─────────────────────────────────────────────> Runtime API :8081
```

**Admin API.** Human-facing. Every route requires an `X-Service-Token` that
exists only in the BFF's server-side environment, so a browser that discovers the
address still cannot use it. Accepts browser sessions only. Never returns
plaintext except from `POST /secrets/{id}/reveal`.

**Runtime API.** Machine-facing. Accepts machine tokens, runtime sessions, and
CLI logins. Deliberately does *not* require the service credential, a workload
authenticates as itself, and shipping a shared service credential to every
container would make one compromised container a foothold on the human API.

Because they are separate listeners, a deployment can put them on separate
networks. The BFF never needs to reach `:8081`, and nothing on `:8081` can reach
the admin routes.

## Request paths

### A developer starts an application

```
ward run -- npm run dev
  │
  ├─ read ~/.warder/credentials.json (mode 0600) or WARDER_TOKEN
  │
  ├─ POST /runtime/auth ──────────────────> resolve identity
  │                                          resolve project + environment
  │                                          mint a 5-minute scoped session
  │  <── vrt_… ────────────────────────────
  │
  ├─ POST /runtime/secrets {keys:[…]} ────> for each requested key:
  │                                            authorize        ← policy engine
  │                                            check deliverable ← expiry, revocation
  │                                            load ciphertext   ← only now
  │                                            decrypt           ← only now
  │                                            audit
  │  <── {"secrets": {...}} ───────────────
  │
  └─ spawn child with values in its environment block
     (WARDER_TOKEN stripped; nothing written to disk; nothing printed)
```

Nothing is decrypted before authorization. A caller authorized for one key in an
environment causes exactly one decryption, not one per secret present.

### An administrator reveals a value

```
Browser ── POST /api/secrets/{id}/reveal ── BFF ── POST /secrets/{id}/reveal ── Core API
                                                                                  │
                                                          audit SECRET_REVEAL_REQUESTED
                                                          authorize READ_SECRET
                                                            └─ denied → audit, 403
                                                          decrypt
                                                          audit SECRET_REVEALED
```

The request is recorded before the decision, so a *denied* reveal is visible to
whoever reviews the trail, often the more interesting event.

## The authorization model

Four things combine, and all four are required:

```
identity + capability + environment + secret
```

**Capabilities never imply one another.** `USE_SECRET` and `READ_SECRET` are
separate, and the gap between them is the product.

**Roles carry management authority only.** No role anywhere confers `USE_SECRET`
or `READ_SECRET`. Both always come from an explicit grant. An owner who has
granted themselves nothing can administer the platform and still not read a
value. An administrator *can* grant themselves `READ_SECRET`; they hold
`MANAGE_ACCESS`, and that act is audited, flagged, and normally time-bounded.
That is the honest version of the claim.

**Credential scope narrows, never widens.** Effective authority is the
intersection of the identity's grants and the presented credential's ceiling.
Scope is checked *before* grants, so a development token never reaches a
production evaluation path at all.

**Deny by default.** `authz.Engine.Authorize` returns a decision, not an error,
so callers must handle denial explicitly. A failure to *load* grants is an error,
never an empty grant list, an outage must not read as "this identity has no
access", and certainly not as the inverse.

## Encryption

```
Key provider (KMS / local keyring)     ← never in the database, never in git
        │  wraps
        ▼
Data encryption key (one per version)  ← used for exactly one encryption
        │  encrypts
        ▼
Ciphertext + nonce + tag               ← secret_material schema
```

Every value is bound by AEAD additional authenticated data to
`org | project | environment | secret | version`. An attacker with database write
access cannot move a production ciphertext into a development secret and have the
broker decrypt it: the binding fails and it reports as a decryption failure.

Each row records the key version that wrapped its data key, so rotation is
incremental: add a new key, point `WARDER_ACTIVE_KEY_VERSION` at it, and existing
ciphertext keeps decrypting under the version it was written with.

All of it lives behind one interface, `crypto.SecretEncryptionService`. No
handler, repository, or service constructs a cipher.

## Storage

Two schemas, so the boundary is grantable rather than notional:

- `public`: organizations, users, projects, environments, secret *metadata*,
  grants, tokens, audit. Everything an operator normally needs to look at.
- `secret_material`: ciphertext and wrapped data keys, keyed by version id, with
  no business metadata at all.

`deploy/sql/roles.sql` gives the reporting role the whole of `public` and nothing
on `secret_material`. The application role gets `SELECT`/`INSERT`/`DELETE` on
secret material but not `UPDATE`, because rotation writes a new version rather
than overwriting one.

The audit trail is append-only, enforced by a trigger *and* by withholding the
privileges. Consequence, stated where it will be noticed: deleting an
organization is a privileged procedure, not an ordinary cascade.

## Extension points

Each is an interface today with one implementation and a documented shape for
the rest.

| Interface | Now | Designed for |
|---|---|---|
| `crypto.KeyProvider` | Local keyring | AWS KMS, GCP KMS, Azure Key Vault / Managed HSM |
| `identity.Provider` | Machine token, runtime session, user session | AWS IAM, OIDC, Kubernetes, GCP, Azure |
| `authz.GrantSource` | PostgreSQL | Any policy store |
| `ratelimit.Limiter` | In-process | Redis or another shared backend |

The identity seam is the important one. Authentication is separated from
authorization precisely so that replacing "a workload holds a bearer token" with
"a workload proves who it is" changes one file and no policy code.

## Related documents

- [Threat model](../security/threat-model.md)
- [Limitations](../security/limitations.md): read this one
- [Key management](../security/key-management.md)
- [Disaster recovery](../security/disaster-recovery.md)
