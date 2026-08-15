# Developer guide

Every command you need, in the order you need them.

- [First run](#first-run) — one time, about five minutes
- [Daily development](#daily-development)
- [Adding and rotating secrets](#adding-and-rotating-secrets)
- [Giving an application access](#giving-an-application-access)
- [Onboarding an AI agent](#onboarding-an-ai-agent)
- [CI and containers](#ci-and-containers)
- [Onboarding and offboarding people](#onboarding-and-offboarding-people)
- [Installing on a team's machines](#installing-on-a-teams-machines)
- [Looking in the database](#looking-in-the-database)
- [Running the tests](#running-the-tests)
- [Troubleshooting](#troubleshooting)
- [Command reference](#command-reference)

---

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

## Daily development

Once, in each repository that needs secrets:

```bash
ward init --project payments-api --env development
```

This writes `.warder.json` naming the project and environment. **Commit it.** It
holds no credentials — only two names — so everyone on the team gets the same
target without passing flags.

Once per machine:

```bash
ward login
```

Then, instead of whatever you ran before:

```bash
ward run -- npm run dev
```

That is the whole workflow. The secrets reach the process; they do not reach
your terminal, your shell history, or a file.

```bash
ward run -- npm test
ward run -- python manage.py migrate
ward run --env staging -- ./deploy.sh
```

### Ask for less

```bash
ward run --key DATABASE_URL --key REDIS_URL -- npm test
```

Naming the keys means the process holds only what it actually uses. If one of
them is not available to you, the command stops before starting anything rather
than failing confusingly ten seconds later.

### See what you have

```bash
ward status                 # who you are signed in as
ward project list
ward secret list            # names, versions, expiry — never values
```

`ward secret list` shows a `VALUE` column full of dots on purpose. You can see
that `DATABASE_URL` exists, that it is at v7, and that it expires in fourteen
days. Seeing what it *is* requires `READ_SECRET`, which no role grants, and it
happens in the dashboard where it can be recorded against a person.

---

## Adding and rotating secrets

In the dashboard: **Projects → your project → Secrets → Add secrets**.

Values are encrypted before they are stored. You will not see one again unless
someone explicitly grants you permission to reveal it — including if you are the
person who typed it in.

### Importing a .env file

The same dialog takes a whole file. Paste one into any **key** field and the
rows fill in: `export` prefixes, comments, quotes, trailing notes, and values
spanning several lines are all understood. Pick the environment once at the top
and everything in the batch goes there.

Only the key field does this. A value field takes what you paste literally,
because that is exactly where a raw credential goes — and a connection string
or a PEM block scattered across rows would be worse than no help at all.

The whole import is one transaction. If any key is malformed or already exists,
nothing is stored — an environment holding half a configuration is worse than one
holding none, because an application will boot on it.

Lines that could not be read are named rather than dropped, so a credential
never goes missing quietly. Each secret still gets its own audit event: "twenty
secrets created" would not answer "when did `STRIPE_KEY` appear".

A key that already exists is refused rather than overwritten. Replacing a value
is a rotation, and rotation is a separate, deliberate act.

### Rotating

**Rotate** on the secret's row. A new version becomes active immediately, and
applications pick it up on their next start. Nothing they hold changes: same
project, same environment, same key.

> Rotating here changes the value **Warder stores**. It does not change the
> credential at the provider. Rotate it at the provider first — generate the new
> database password, the new API key — then paste the new value here. The
> dialog says this too, because assuming otherwise leaves a live credential in
> circulation.

### Expiry

Set **Expires** on any version. After it passes, runtimes stop receiving it and
the dashboard shows `EXPIRED`. Useful for a credential you know is temporary; a
loud failure beats a credential that quietly outlives its purpose.

---

## Giving an application access

Three steps, and the order matters.

**1. Create an identity** — *Identities → New identity*

One per thing that runs your code: the API in production, the CI job, an agent.
Separate identities mean you can revoke one without disturbing the others. A new
identity holds no access at all.

**2. Grant it access** — *your project → Access → Grant access*

Choose the identity, the environment, and the capabilities. Two checkboxes:

- **Can use** — a runtime receives the value and injects it into a process.
- **Can see** — a person can display the plaintext in the dashboard.

Applications want the first. They almost never want the second.

**3. Issue it a token** — *your project → Tokens → New token*

The token is shown once. Only a verifier is stored, so it cannot be shown again —
if it is lost, revoke it and issue another.

Then, in the runtime's own environment:

```bash
WARDER_RUNTIME_URL=http://127.0.0.1:8081
WARDER_TOKEN=vlt_...
```

```bash
ward run -- ./server
```

Same command as a developer runs. The credential comes from the environment
instead of a login.

### Answering "what happens if I revoke this?"

Immediately. Revoking a token also revokes every short-lived session already
issued from it, so the next request is denied rather than the one after the
session would have expired.

---

## Onboarding an AI agent

An agent is not you. It reads untrusted input — issues, pull requests,
dependency READMEs — and can be talked into things. Give it its own identity.

*Identities → New identity → **AI agent***, and set an expiry. The session ends
by itself; nobody has to remember to clean it up.

Then grant it **development**, **can use only**, and issue a token scoped to the
keys it needs:

```bash
WARDER_RUNTIME_URL=http://127.0.0.1:8081
WARDER_TOKEN=vlt_...
```

The agent can now run:

```bash
ward run -- npm test
```

and cannot print a credential, reach staging or production, rotate anything, or
change who has access. The dashboard API refuses its token outright.

What this does **not** do: `ward run -- npm test` starts a process whose
environment holds the test credentials, and the agent can read that process's
environment. The protection is that those are test credentials, in development,
scoped as narrowly as you chose — not that the agent is prevented from reading
what it was authorized to use. See [limitations](security/limitations.md).

---

## CI and containers

Create a **CI** identity, grant it the environment it deploys to, issue a token,
and store that token in your CI system's secret store. Then:

```yaml
- run: ward run -- ./deploy.sh
  env:
    WARDER_RUNTIME_URL: ${{ vars.WARDER_RUNTIME_URL }}
    WARDER_TOKEN: ${{ secrets.WARDER_TOKEN }}
```

One credential in your CI system instead of fifteen, and it is scoped to one
project and one environment. Rotating a database password no longer means
editing CI configuration.

In a container, set the same two variables and use `ward run` as the entrypoint:

```dockerfile
ENTRYPOINT ["ward", "run", "--"]
CMD ["./server"]
```

---

## Onboarding and offboarding people

**Adding someone:** *Members → Invite member*. Give their name, address and
role, and set an expiry if they are a contractor. You get a single-use link to
send them; nothing is emailed from Warder.

The link works once and lapses after seven days. Until it is used you can
withdraw it from the same page, which is what to do if it went to the wrong
place.

**Warder never asks you to choose somebody else's password.** The invitee sets
their own when they open the link, so you do not learn it, you never have to
send a credential over chat, and you cannot sign in as them afterwards. The
address and the role are fixed when you create the invitation — whoever opens
the link joins as exactly that person, at exactly that role, and can change
neither.

The role governs administration. It grants no access to any secret value — that
is always a separate, explicit grant.

**Removing someone:** *Members*, then remove them. Their sessions stop working
on the next request, and their password stops conferring anything even though it
is still correct.

**No credential needs rotating.** That is the point. Someone who could *use*
`DATABASE_URL` never held it, so their leaving does not put it at risk.

---

## Installing on a team's machines

Developers who are not working on Warder itself should not have to clone it and
have a Go toolchain to get the CLI. Three ways in, in the order most people
want them.

### The install script

```bash
curl -fsSL https://raw.githubusercontent.com/Tobe0504/Warder/main/install.sh | sh
```

Detects the platform, downloads the matching build from the GitHub release,
**verifies its SHA-256 against the published checksums**, and installs to the
first writable directory on PATH. Pin a version or choose a location:

```bash
WARD_VERSION=v0.2.0 WARD_INSTALL_DIR="$HOME/.local/bin" ./install.sh
```

The checksum check is not decoration. This binary ends up holding a credential
that reaches every secret its identity is granted, so a tampered download is
not a cosmetic problem. The script refuses to install on a mismatch.

> Piping a script into a shell is a real thing to be uneasy about. If your
> team's policy says no, download it, read it, then run it — it is a hundred
> lines of POSIX sh and does nothing but fetch, verify, and move a file.

### With Go

```bash
go install github.com/Tobe0504/Warder/cmd/ward@latest
```

Lands in `$(go env GOPATH)/bin`. The version reports as `0.1.0-dev`, because
the real tag is stamped by the release build rather than compiled in.

### Straight from a release

Download the archive for the platform from the releases page, check it against
`checksums.txt`, extract, and put `ward` somewhere on PATH.

### Publishing a release

Tag and push. The release workflow cross-compiles for macOS and Linux on both
architectures, publishes the checksums, and creates the GitHub release.

```bash
git tag v0.2.0 && git push origin v0.2.0
```

Binaries are built with `CGO_ENABLED=0` and `-trimpath`, so each one is static
— no libc to match — and carries no local filesystem paths.

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

## Troubleshooting

### "Could not reach the service"

The dashboard is up; the core API is not.

```bash
docker compose -f deploy/docker-compose.yml ps    # is Postgres up?
curl -s http://127.0.0.1:8081/health              # is the API up?
```

### "Not configured yet" on the sign-in page

`web/.env.local` is missing or `WARDER_URL` is malformed. The page prints
exactly what is wrong. Regenerate with `go run ./cmd/warder-api init`.

### The API will not start

It validates configuration before binding anything and names what is missing.
The usual causes are an unset `WARDER_KEYRING` or a `WARDER_SERVICE_TOKEN`
shorter than 32 characters.

### `ward: not logged in`

```bash
ward login
```

Or, for a machine runtime, set `WARDER_RUNTIME_URL` and `WARDER_TOKEN` in its environment.

### `ward run` says "not authorized"

Your identity has not been granted `USE_SECRET` on that environment. Check the
project's **Access** tab. Being an owner is not enough — no role grants access
to secret values.

### A secret shows `EXPIRED`

Its active version passed its expiry. Rotate it, or clear the expiry from the
version list.

### The dashboard renders but nothing is clickable

Almost always a Content Security Policy problem. Check the browser console for
policy violations. Development allows `unsafe-eval` because Next.js needs it for
hot reloading; production does not.

---

## Command reference

### `warder-api`

| Command | What it does |
|---|---|
| `warder-api init` | Generate a complete starting configuration |
| `warder-api migrate` | Apply database migrations |
| `warder-api serve` | Run the admin and runtime listeners |
| `warder-api keygen` | Print one new key encryption key |

### `ward`

| Command | What it does |
|---|---|
| `ward init --project P --env E` | Write `.warder.json` — commit it |
| `ward login` | Sign in and store a session at mode 0600 |
| `ward logout` | Revoke the session and remove it locally |
| `ward status` | Who you are signed in as |
| `ward project list` | Projects you can see |
| `ward environment list` | Environments in a project |
| `ward secret list` | Secret names, versions, expiry — never values |
| `ward run -- <cmd>` | Run with authorized secrets injected |

Flags for `ward run`:

| Flag | Effect |
|---|---|
| `--project` | Override `.warder.json` |
| `--env` | Override `.warder.json` |
| `--key KEY` | Request only this secret; repeatable |
| `--quiet` | Suppress the summary line |

### Environment variables

For a runtime, instead of `ward login`:

| Variable | Purpose |
|---|---|
| `WARDER_RUNTIME_URL` | The runtime API address |
| `WARDER_TOKEN` | The machine token |

`ward run` removes `WARDER_TOKEN` from the child process's environment. The
child needs the secrets, not the ability to ask for more of them.
