# Warder

A secret access broker. Applications get the credentials they need; the people
and agents who build those applications do not.

```
Developer                    ward run -- npm run dev
    │                                 │
    │  never sees DATABASE_URL        │  process receives DATABASE_URL
    ▼                                 ▼
```

This is not an encrypted `.env` manager. Encrypted storage, versioning, and CLI
injection are table stakes that Doppler, Infisical, 1Password, and Vault already
do well. The thing Warder is built around is narrower:

> **Using a credential and seeing a credential are different permissions.**

A developer can run the application. An AI agent can run the tests. CI can
deploy. None of them are handed the value, and none of them need to be.

---

## What that buys you

| Question | Answer |
|---|---|
| A contractor leaves. What now? | Revoke their membership. No credential rotates. |
| An agent's session is prompt-injected. | It holds `USE_SECRET` on development. It cannot print a value, reach production, or grant itself anything. |
| Someone leaked a `.env`. | There wasn't one. |
| Who can see this credential? | One query. Usually: nobody. |
| What happens if I revoke this token? | The next request is denied, including sessions already derived from it. |

---

## Quick start

```bash
docker compose -f deploy/docker-compose.yml up -d
```

```bash
go run ./cmd/warder-api init
```

That prints two blocks: save the first as `.env`, and the `WARDER_URL=` line as
`web/.env.local`. It generates the encryption key, the service credential, and
the dashboard's connection URI together, so they already agree.

```bash
set -a && source .env && set +a
go run ./cmd/warder-api migrate
go run ./cmd/warder-api serve
```

The dashboard, in a second terminal:

```bash
cd web && npm install && npm run dev
```

Open <http://localhost:3000>, create an organization, and add a project and a
secret. Then, from your application's directory:

```bash
go build -o /usr/local/bin/ward ./cmd/ward
ward init --project payments-api --env development
ward login
ward run -- npm run dev
```

That last command starts your application with `DATABASE_URL` in its
environment. Nothing was written to `.env`, nothing was printed, and you never
saw the value.

Step-by-step: **[docs/developer-guide.md](docs/developer-guide.md)**.

### One variable for the dashboard

The BFF needs to know where the core API is, the credential that proves it *is*
the BFF, which posture to run in, and which origin the browser uses. Those
travel together as one connection URI, in the shape you already know from
database connection strings:

```
WARDER_URL=warder+insecure://<service-token>@127.0.0.1:8080/development
WARDER_URL=warder://<service-token>@api.internal:8443/production?origin=https://vault.acme.com
```

Four separate variables could be configured three-quarters of the way, and the
result: a production address with a stale token: fails at the first request in
a way that looks like a network problem. One value either parses completely or
the process refuses to start. It also means the token cannot be left pointing at
the wrong endpoint, and `warder+insecure://` with `/production` is a startup
error rather than a quiet downgrade to unencrypted transport.

It is not cryptographically stronger than four variables. It is much harder to
misconfigure, which is where the failures actually come from.

---

## How it fits together

```
Browser ──HTTPS──> Next.js BFF ──service credential──> Admin API   :8080
                                                          │
Runtime / CLI ───────────────────────────────────────> Runtime API :8081
                                                          │
                                    policy engine ── encryption ── PostgreSQL
                                                          │
                                                    key provider (KMS)
```

Two listeners, deliberately. The browser can never reach the core API: every
admin route requires a service credential that exists only on the BFF's server
side. The runtime API accepts workload credentials and requires no shared secret,
because shipping one to every container would make one compromised container a
foothold on the human-facing API.

Full detail in [docs/architecture/overview.md](docs/architecture/overview.md).

### Capabilities

```
READ_METADATA    this secret exists, it is at v7, it expires in 14 days
USE_SECRET       a runtime may receive the value
READ_SECRET      a human may see the value
CREATE_SECRET  ROTATE_SECRET  REVOKE_SECRET  MANAGE_ACCESS  READ_AUDIT
```

`USE_SECRET` never implies `READ_SECRET`. **No role confers either one**: both
always come from an explicit, audited, usually time-boxed grant. An owner who has
granted themselves nothing can administer the platform and still not read a
value.

An administrator *can* grant themselves `READ_SECRET`; they hold `MANAGE_ACCESS`.
That act is recorded and flagged. This is the honest version of the claim, and
the reason the docs never say "developers can never access secrets".

---

## The CLI

```bash
ward login                              # session stored at mode 0600
ward project list
ward secret list                        # names, versions, expiry: never values
ward run -- npm run dev                 # the one that matters
ward run --key DATABASE_URL -- npm test # ask for less, expose less
```

For CI and containers, set `WARDER_RUNTIME_URL` and `WARDER_TOKEN` in the runtime's own
environment; the command is otherwise identical.

`ward run` does not print values, does not write them to disk, does not put them
in argument vectors, and strips `WARDER_TOKEN` from the child so a compromised
process cannot ask for more than it was given.

It cannot stop the program it starts from doing any of those things. See
[limitations](docs/security/limitations.md).

---

## Tests

```bash
go test ./...                                  # unit tests; integration ones skip
export WARDER_TEST_DATABASE_URL="postgres://warder:warder-local-dev-only@127.0.0.1:5432/warder?sslmode=disable"
go test ./...                                  # everything, against real Postgres
```

For the dashboard:

```bash
cd web && npm run verify
```

That runs the boundary checks, which fail on `NEXT_PUBLIC_`, on a client
component reaching a server module, on `dangerouslySetInnerHTML`, and on browser
storage: then typecheck and unit tests.

The suite in `internal/apitest` is the part worth reading. It is written to fail
when a guarantee breaks, not to cover lines:

- a development token reaching production
- a token from project A reaching project B
- an agent identity using a secret but not revealing it, and not inheriting its
  creator's access
- a revoked token's *already-issued* sessions still working
- a value appearing in logs, in an error body, or in any dashboard response
- a corrupted ciphertext producing an error that describes the failure
- ciphertext relocated between rows in the database decrypting anywhere
- a contractor's access ending without any credential rotating
- the real `ward` binary injecting into a real child process

---

## Documentation

| | |
|---|---|
| [**Developer guide**](docs/developer-guide.md) | **Every command, in the order you need it** |
| [Architecture](docs/architecture/overview.md) | Surfaces, request paths, extension points |
| [Threat model](docs/security/threat-model.md) | Actors, boundaries, mitigations, known gaps |
| [**Limitations**](docs/security/limitations.md) | **What this does not do: read this** |
| [Key management](docs/security/key-management.md) | Envelope encryption, rotation, exposure response |
| [Disaster recovery](docs/security/disaster-recovery.md) | Restore, and what a restore silently undoes |

---

## Status

MVP. The engine: encryption, authorization, identity, delivery, audit, is
built and tested. It has not been audited, certified, or assessed against any
compliance framework, and several controls a production deployment needs are
absent and listed in [limitations](docs/security/limitations.md). Read that page
before putting a real credential in this.
