# Deploying Warder

Warder is three things: a Go API, a Postgres database, and a Next.js
dashboard. This walks through putting them on Render and Vercel, which is the
path with the fewest moving parts.

- [What goes where, and why](#what-goes-where-and-why)
- [Before you start](#before-you-start)
- [1. Generate the keys](#1-generate-the-keys)
- [2. Deploy the API and database](#2-deploy-the-api-and-database)
- [3. Create the schema](#3-create-the-schema)
- [4. Deploy the dashboard](#4-deploy-the-dashboard)
- [5. Point the CLI at it](#5-point-the-cli-at-it)
- [What to check afterwards](#what-to-check-afterwards)
- [Things this deployment does not do](#things-this-deployment-does-not-do)

---

## What goes where, and why

Warder serves two HTTP surfaces, and keeping them apart is most of the design.

| Surface | Who talks to it | What it trusts |
|---|---|---|
| **Admin** | Only the dashboard's backend | A service credential the dashboard holds |
| **Runtime** | The `ward` CLI and deployed workloads | Each workload's own token: no shared credential |

They are separate because collapsing them would mean shipping the service
credential to every workload, and one stolen container would then be a foothold
on the human-facing API.

Render gives a service one public port, so this runs **two services from one
image**. Each publishes its own surface and leaves the other on loopback, where
nothing outside the container can reach it. That is what
[`render.yaml`](../render.yaml) declares.

```
  Browser ──► Vercel (dashboard + BFF) ──► Render: warder-admin ──┐
                                                                  ├──► Postgres
  ward CLI, your apps ─────────────────► Render: warder-runtime ──┘
```

> **One consequence worth understanding.** Putting the dashboard on Vercel
> means its backend reaches the admin surface across the public internet, so
> the admin port has to be publicly addressable, protected by the service
> token and nothing else. Running the dashboard on Render instead would let it
> use the private network and the admin port could stay unreachable. The
> Vercel path is simpler; this is what it costs.

## Before you start

You need a GitHub account with this repository, a Render account, and a Vercel
account. All three have free tiers that fit this.

Install the API binary locally first; you will use it to generate keys and to
run the migration:

```bash
go build -o "$(dirname "$(command -v go)")/warder-api" ./cmd/warder-api
```

Or name a directory yourself; it has to be one that is already on your PATH,
not merely one that exists. On a Mac with Homebrew, `/opt/homebrew/bin` is the
reliable answer; `~/.local/bin` exists on plenty of machines that never put it
on PATH.

```bash
go build -o /opt/homebrew/bin/warder-api ./cmd/warder-api
```

Check it before going on:

```bash
warder-api
```

That prints the list of commands. `command not found` means the directory you
chose is not on your PATH, `echo $PATH` will show you which ones are.

## 1. Generate the keys

Two secrets, generated once:

```bash
warder-api keygen
```

That prints a key encryption key. Every secret Warder stores is encrypted under
it.

> **This is the one thing you cannot regenerate.** Lose it and every stored
> secret is unrecoverable; there is no reset, by design. Put a copy somewhere
> durable and separate from the database before you go further. See
> [key management](security/key-management.md) and
> [disaster recovery](security/disaster-recovery.md).

And a service credential, at least 32 characters, from a real random source:

```bash
openssl rand -hex 32
```

**Hex, not base64.** This value goes into a URI, and base64's alphabet includes
`/`, which ends the authority before the `@` is reached. The token would never
arrive and the error would point at the wrong half of the string. Hex is
alphanumeric, so it survives a URI, a shell, and a dashboard form unchanged.

Keep both somewhere you can paste from in the next step. Not in the repository,
not in a chat message.

## 2. Deploy the API and database

In Render: **New → Blueprint**, pick this repository. Render reads
`render.yaml` and proposes a database and two services.

It will stop and ask for the values marked `sync: false`. Set the **same value
on both services**:

| Variable | Value |
|---|---|
| `WARDER_KEYRING` | the key from `warder-api keygen` |
| `WARDER_SERVICE_TOKEN` | the string from `openssl rand -hex 32` |

`WARDER_DATABASE_URL` is wired automatically to the managed database's internal
URL, which does not leave Render's network. You do not set it.

Both services read the same ciphertext, so they need the same keyring. The
service token is only *used* by the admin surface, but the binary requires it
at startup on both.

Apply. Render builds the image once and starts both services:

```
https://warder-admin-XXXX.onrender.com      the dashboard's backend talks here
https://warder-runtime-XXXX.onrender.com    the ward CLI talks here
```

<!-- TODO: replace with the real hostnames once the first deploy is done. -->

Check both:

```bash
curl -s https://warder-runtime-XXXX.onrender.com/health
```

`{"status":"ok"}` from each. The admin service answers the same on `/health`
and refuses everything else without the service token, which is correct.

> **The free database is removed after 30 days.** Fine for evaluating, and a
> poor property for the store holding every secret an organization owns. See
> [using a different database](#using-a-different-database) when you outgrow
> it: nothing about Warder is tied to Render.

## 3. Create the schema

**Do not skip this.** Render creates an empty database; the services start fine
against it and then every request fails, because there are no tables. The API
answers `internal_error`: deliberately, since a caller learns nothing about
the internals, and the real cause appears only in the service log as
`relation "users" does not exist`.

Copy the **External Database URL** from the Render database page. The services
use the internal one; this command runs from your machine, so it needs the
external.

```bash
WARDER_DATABASE_URL="postgres://…" warder-api migrate
```

That is the only variable it needs. A schema change has no business requiring
the keyring, and demanding it would put the key that decrypts every secret in
one more shell for no benefit.

Expect `applied 0001_init.sql` and `applied 0002_invitations.sql`. Run it again
after any deploy that adds a migration; when there is nothing to apply it says
`Database is up to date.` and exits.

## 4. Deploy the dashboard

In Vercel: **Add New → Project**, pick this repository, and set the **root
directory to `web`**. Vercel detects Next.js on its own.

There is deliberately no `vercel.json`. Everything one would put in it is
either Vercel's default or already handled in `middleware.ts`, which sets the
security headers per request so the Content-Security-Policy can carry a fresh
nonce. A second place to configure headers is a second place for them to drift.

Two environment variables:

```
WARDER_URL=warder://<service-token>@warder-admin-XXXX.onrender.com:443/production?origin=https://<your-vercel-domain>
WARDER_PUBLIC_RUNTIME_URL=https://warder-runtime-XXXX.onrender.com
```

That single URI carries everything the dashboard needs: the scheme states the
transport, the userinfo is the service token, the path is the posture, and
`origin` is the address browsers will announce. See
[connection.ts](../web/lib/connection.ts) for the full grammar.

Three details that will bite if you get them wrong:

- **`warder://` not `warder+insecure://`.** The plain scheme means HTTPS.
  Production refuses the insecure one.
- **Port `443`.** Render serves HTTPS on the standard port; the URI needs it
  stated.
- **`origin` is required in production.** It is what lets the BFF refuse
  cross-site requests. Set it to your real Vercel domain, including `https://`.

`WARDER_PUBLIC_RUNTIME_URL` is the **runtime** host, not the admin one, and it
is not a credential: it is the address the dashboard prints into the setup
commands on every environment page, so that someone who did not deploy Warder
can copy a `ward init` that works. Leave it unset and the dashboard prints a
bare `ward init`, which points the CLI at `127.0.0.1:8081` and fails with a
connection error the first time anyone runs `ward run`.

Deploy. Open the domain and choose **Create an organization**.

## 5. Point the CLI at it

With `WARDER_PUBLIC_RUNTIME_URL` set above, developers do not have to know the
address at all: the dashboard hands them a `ward init` that records it in
`.warder.json`, which is committed, so everyone who clones the repository picks
it up and `ward login` finds it without a flag.

The address can also be given directly, which is what CI does:

```bash
export WARDER_RUNTIME_URL=https://warder-runtime-XXXX.onrender.com
ward login
```

For deployed workloads, the runtime URL and a machine token go into the
platform's secret store. See
[using Warder in your application](using-warder.md).

## What to check afterwards

Worth doing once, in this order, because each proves a different boundary:

1. **The admin surface refuses anonymous callers.**
   `curl https://warder-admin-XXXX.onrender.com/projects` → `service_unauthorized`.
2. **The runtime surface does not accept the service token.** It has no
   service-token middleware at all; a workload authenticates as itself.
3. **A token with no grant gets nothing.** Create an identity and a token
   *without* granting access, then `ward run`. It should authenticate and
   deliver zero secrets, naming what it was refused.
4. **Sign-out ends the session.** The next request should redirect to sign-in
   rather than serving a cached page.

## Using a different database

Nothing about Warder is tied to Render's database. To point at an external one:
when the free tier expires, or for anything real, delete the `databases`
block from `render.yaml` and change both `WARDER_DATABASE_URL` entries from
`fromDatabase` to `sync: false`, then set the connection string on each service.

It has to be **PostgreSQL**, not a Postgres-flavoured API. Warder uses two
schemas: `public` for metadata and `secret_material` for ciphertext, and that
split is what lets a reporting role be given everything about which credentials
exist while holding no privilege on the ciphertext. It also uses `bytea`,
`jsonb`, `timestamptz`, a trigger that makes audit rows unrewritable, and
session-level advisory locks so two instances cannot apply the same migration
twice.

SQLite-backed services such as Turso cannot express that. Schemas do not exist
there, so the separation the threat model rests on would simply be gone.

[Neon](https://neon.tech) is the natural next step: real Postgres, a free tier
that does not expire, suspended when idle rather than deleted. Supabase, Aiven
and Railway work too. If the provider offers a connection pooler, use the
pooled string for the services and the **direct** one for migrations:
session-level advisory locks do not survive a transaction-mode pooler.

---

## Things this deployment does not do

Stated plainly, because a deployment guide that only lists successes is not
useful.

**The free tiers sleep and expire.** Render free services spin down when idle,
so the first `ward run` after a quiet period waits for a cold start, and the
free database is removed after 30 days. Fine for evaluating; unsuitable for a
workload that needs a credential in under a second, or for data you intend to
keep.

**The keyring sits in an environment variable.** That is the local key
provider, and it means anyone who can read the service's configuration can read
the key that decrypts everything. For production, move to a KMS so the key
never leaves the provider, the `KeyProvider` interface exists for exactly this
and the cloud implementations are stubbed in
[`internal/crypto/kms_cloud.go`](../internal/crypto/kms_cloud.go). See
[key management](security/key-management.md).

**Organization creation is unauthenticated.** Anyone who can reach the
dashboard can create a tenant. Gate this before you put it anywhere public:
it is the first item in [limitations](security/limitations.md).

**Nothing backs up the database.** Render's free tier does not, and a Warder
database without its keyring is unrecoverable ciphertext. Read
[disaster recovery](security/disaster-recovery.md) before you rely on this.

**The database roles are not applied.** `deploy/sql/roles.sql` splits migrator,
application, and read-only privileges so that a leaked reporting credential
exposes no ciphertext. Render's default user owns everything. Applying it is a
deliberate step: read it first, because it reassigns table ownership.
