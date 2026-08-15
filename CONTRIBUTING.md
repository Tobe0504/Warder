# Contributing

Working on Warder itself. Everything here is about running the project locally;
using a deployed Warder is [the developer guide](docs/developer-guide.md).

This file is deliberately not part of the published documentation. Someone
reading the docs site wants to know how to use the product, not how to start a
Postgres container.

## First run

### 1. Start the database

```bash
docker compose -f deploy/docker-compose.yml up -d
```

Postgres only. There is no Redis: plaintext secrets are never cached, so there
is nothing for it to do.

### 2. Generate configuration

```bash
go run ./cmd/warder-api init
```

This prints two blocks. It generates the encryption key, the service
credential, and the dashboard's connection URI together, so they agree with each
other — which is the part that is tedious and easy to get wrong by hand.

Save the first block as `.env` in the repository root, and the line beginning
`WARDER_URL=` as `web/.env.local`. Both are git-ignored.

> The keyring is the one thing you cannot regenerate. Lose it and every secret
> encrypted under it is gone. For anything beyond local development, put it in a
> KMS — see [key management](security/key-management.md).

### 3. Create the schema

```bash
set -a && source .env && set +a
go run ./cmd/warder-api migrate
```

### 4. Start the API

```bash
go run ./cmd/warder-api serve
```

Two listeners come up: `127.0.0.1:8080` for the dashboard's backend, and
`127.0.0.1:8081` for runtimes. They are separate so a deployment can put them on
separate networks.

A quick check, in another terminal:

```bash
curl -s http://127.0.0.1:8081/health
```

`{"status":"ok"}`. The admin port answers `401` to the same request, which is
correct — it requires the service credential that only the dashboard holds.

### 5. Start the dashboard

```bash
cd web && npm install && npm run dev
```

Open <http://localhost:3000>, choose **Create an organization**, and you are in.

### 6. Install the CLI

From this checkout:

```bash
go build -o "$HOME/.local/bin/ward" ./cmd/ward
```

Not `/usr/local/bin` — it is root-owned on Apple Silicon, so that build fails
silently and leaves you with `command not found`. Use a directory you own that
is already on your PATH.

See [installing on a team's machines](#installing-on-a-teams-machines) for
everyone who is not working on Warder itself.

---

## Looking in the database

Useful for understanding what is actually stored, and for convincing yourself
the separation is real.

Local connection details come from `deploy/docker-compose.yml`:

| Field | Value |
|---|---|
| Host | `127.0.0.1` |
| Port | `5432` |
| Database | `warder` |
| User | `warder` |
| Password | `warder-local-dev-only` |

Development-only credentials, and the container is bound to loopback so nothing
on the local network can reach it.

### The two schemas

`public` holds metadata: who exists, what secrets exist, who may use them.
`secret_material` holds ciphertext and wrapped data keys, and nothing else.

That split is the point. A reporting tool, a support dashboard, or an analytics
replica can be given the whole of `public` while holding no privilege at all on
`secret_material`.

### Seeing that encryption is doing something

```sql
SELECT s.key, v.version, v.encryption_key_id,
       length(m.ciphertext) AS ct_bytes,
       encode(substring(m.ciphertext from 1 for 12), 'hex') AS first_bytes
FROM secrets s
JOIN secret_versions v ON v.secret_id = s.id
JOIN secret_material.secret_version_material m ON m.secret_version_id = v.id
ORDER BY s.key;
```

Two rows holding the *same* value produce completely different ciphertext:
every version gets its own data key and its own nonce. There is no way to tell
from this table which secrets share a value.

### Connect as the read-only role instead

Browsing as the owning user shows you everything, which is not how anyone
should be looking at a production database. `deploy/sql/roles.sql` creates
`warder_readonly`, which has no privilege on `secret_material` at all, and
cannot select password hashes or credential verifiers.

Connect DBeaver as that role and the ciphertext tables are simply not there —
which is the most direct demonstration of the model there is.

> Read that file before running it. It reassigns ownership of every table to
> `warder_migrator`, so an API still connecting as the old user will stop
> working until its connection string is updated. It is a production step, not
> something to run against a local database you are in the middle of using.

---

## Running the tests

```bash
go test ./...
```

Integration tests skip themselves without a database. With one:

```bash
export WARDER_TEST_DATABASE_URL="postgres://warder:warder-local-dev-only@127.0.0.1:5432/warder?sslmode=disable"
go test -race ./...
```

The suite in `internal/apitest` is the interesting part — it is written to fail
when a guarantee breaks: a development token reaching production, a value
appearing in a log, a revoked token's existing sessions still working.

For the dashboard:

```bash
cd web
npm run verify     # boundary checks, typecheck, unit tests
npm run build
```

`npm run check` alone is the fast one. It fails on `NEXT_PUBLIC_`, on a client
component reaching a server module, on `dangerouslySetInnerHTML`, and on browser
storage.

---

## Releasing

Tag and push. The release workflow cross-compiles the CLI for macOS and Linux
on both architectures, publishes checksums, and creates the GitHub release.

```bash
git tag v0.2.0 && git push origin v0.2.0
```

Binaries are built with `CGO_ENABLED=0` and `-trimpath`, so each one is static
and carries no local filesystem paths. The version is stamped by the linker —
which is why `internal/cli.Version` is a `var` and not a `const`.
