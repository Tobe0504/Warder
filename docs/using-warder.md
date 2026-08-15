# Using Warder in your application

This guide is for the person whose application has a `.env` file full of
credentials, who wants to know what actually changes.

The short version: **your code does not change.** You keep reading
`process.env.DATABASE_URL`. What changes is where that value comes from at the
moment your process starts, and what is left sitting on disk when it isn't
running.

- [The one thing to understand first](#the-one-thing-to-understand-first)
- [What stays in your environment](#what-stays-in-your-environment)
- [Where the two values come from](#where-the-two-values-come-from)
- [Frontends: what Warder can and cannot do](#frontends-what-warder-can-and-cannot-do)
- [Worked example: a Next.js application](#worked-example-a-nextjs-application)
- [Build time is not run time](#build-time-is-not-run-time)
- [Where this works, and where it doesn't](#where-this-works-and-where-it-doesnt)
- [What your process actually receives](#what-your-process-actually-receives)
- [Questions people ask](#questions-people-ask)

---

## The one thing to understand first

Warder does not get you to zero credentials. It gets you to **one**.

Your application still needs something that proves *which application it is*.
Without that, Warder has no way to tell your API server from anyone else who can
reach the port. That one credential is `WARDER_TOKEN`, and the problem it
represents has a name — "secret zero". Nobody has solved it. HashiCorp Vault,
AWS Secrets Manager, Doppler and every other broker have exactly the same
bootstrap step.

What changes is the shape of the risk:

|  | Before | With Warder |
| --- | --- | --- |
| Credentials on disk | 5–20, all long-lived | 1 |
| Scope of each | Usually full production access | One project, one environment, optionally specific keys |
| Expiry | None | Optional, and enforced |
| Rotating one | Edit every `.env`, every CI config, redeploy | Rotate in the dashboard; running apps pick it up on restart |
| Someone leaves | Rotate everything they could have copied | Revoke their membership. Nothing to rotate |
| Who saw what | Unknowable | Recorded per access |

The last two rows are the point of the product. Someone who could *use*
`DATABASE_URL` through Warder never actually held it, so their leaving does not
put it at risk.

---

## What stays in your environment

Exactly two variables, and only one of them is secret.

```bash
WARDER_RUNTIME_URL=https://warder.your-company.com    # an address. Not secret.
WARDER_TOKEN=vlt_...                          # the one credential. Secret.
```

Plus a file, which you **commit**:

```jsonc
// .warder.json — written by `ward init`
{ "project": "payments-api", "environment": "production" }
```

It holds two names and no credentials. Committing it means everyone on the team
and every deployment targets the same place without passing flags.

### On a developer's laptop, it's zero

A developer runs `ward login` once per machine. The credential lands in their
home directory, readable only by them, and they set **no environment variables
at all**:

```bash
ward run -- npm run dev
```

`WARDER_RUNTIME_URL` and `WARDER_TOKEN` are the *machine* path — CI, containers, servers.

### Where to put `WARDER_TOKEN`

In order of preference:

1. **Your platform's secret store.** Fly secrets, an ECS task secret, a
   Kubernetes secret, a GitHub Actions secret. This is the intended home.
2. A `.env` file that is git-ignored. Works, but it is the one thing that can
   still end up in a commit or baked into a Docker layer.

Never in the repository, never in a `Dockerfile`, never in a shell command
(where it lands in history and in `ps`).

---

## Where the two values come from

### `WARDER_TOKEN` — from the dashboard

**Your project → Tokens → New token.**

Give it a name, pick the identity it belongs to, pick the environment, and
optionally narrow it to specific secret keys. The token is displayed **once**,
on the confirmation screen, with a copy button.

Only a verifier is stored on the server, so it genuinely cannot be shown again.
If it is lost, revoke it and issue another — that is a thirty-second operation,
not an incident.

Before you can issue a token, the identity needs to exist and needs a grant.
Three steps, covered in [the developer guide](developer-guide.md#giving-an-application-access):

1. **Identities → New identity** — one per thing that runs your code
2. **Project → Access → Grant access** — choose *Can use*, not *Can see*
3. **Project → Tokens → New token**

### `WARDER_RUNTIME_URL` — from your deployment, not the dashboard

This is the address of your Warder deployment's **runtime listener**. The
dashboard doesn't display it, because the dashboard has no way to know what
hostname you put in front of your own infrastructure.

It's set when the API is deployed:

```bash
WARDER_RUNTIME_ADDR=127.0.0.1:8081
```

So:

- **Locally:** `WARDER_RUNTIME_URL=http://127.0.0.1:8081`
- **Deployed:** `https://` plus whatever hostname fronts that port

> **This is a different port from the dashboard's API** (`:8080`). The two
> surfaces are deliberately separate: the runtime port does not accept the
> dashboard's service token, and the admin port does not accept runtime tokens.
> Pointing an application at `:8080` fails authentication rather than
> half-working.

---

## Frontends: what Warder can and cannot do

"Frontend" covers two different things, and they get opposite answers.

### Anything the browser receives is not a secret

`NEXT_PUBLIC_*`, `VITE_*`, `REACT_APP_*` — these are compiled into the
JavaScript bundle and shipped to every visitor. Anyone can open devtools and
read them.

**Warder does not manage these, and no broker can.** A value you hand to the
public is public. They stay in your `.env` exactly as they are today.

They are usually not real secrets anyway: a Stripe *publishable* key, a PostHog
project key, a public API base URL. They are designed to be visible.

> **The one rule that matters here:** never move a Warder-managed secret into a
> `NEXT_PUBLIC_*` variable. That publishes it to every visitor, and nothing
> upstream can undo it. If a value needs to stay private, the code that uses it
> has to run on the server.

### The server half is what Warder takes over

In a modern framework, most of your application is server code — route
handlers, server actions, server components, loaders. `DATABASE_URL`,
`STRIPE_SECRET_KEY`, `AUTH_SECRET` all live there, and never reach the browser.

Those go into Warder. Your code keeps reading them from `process.env`. There is
no SDK to install and no import to add.

---

## Worked example: a Next.js application

### Before

`.env.local`, git-ignored, and the file everyone quietly worries about:

```bash
DATABASE_URL=postgres://user:pass@db.internal:5432/payments
STRIPE_SECRET_KEY=sk_live_51H...
AUTH_SECRET=8f3c9a...
RESEND_API_KEY=re_9dK2...
NEXT_PUBLIC_STRIPE_KEY=pk_live_51H...
NEXT_PUBLIC_POSTHOG_KEY=phc_a1b2...
```

### After

```bash
NEXT_PUBLIC_STRIPE_KEY=pk_live_51H...
NEXT_PUBLIC_POSTHOG_KEY=phc_a1b2...
```

Two public values. Nothing in the file is worth stealing, so the file stops
being a liability.

Next to it, committed:

```jsonc
// .warder.json
{ "project": "payments-api", "environment": "development" }
```

The four real credentials now live in Warder and arrive at the process when it
starts.

### Running it

**Locally**, after `ward login` once:

```bash
ward run -- npm run dev
```

**Deployed**, with `WARDER_RUNTIME_URL` and `WARDER_TOKEN` in the platform's secret store:

```bash
ward run -- npm start
```

Same shape in both places. That is the entire integration.

### Asking for less

If a particular command only needs two of the four, say so:

```bash
ward run --key DATABASE_URL --key REDIS_URL -- npm test
```

The process then holds only what it uses. If one of those keys isn't available
to you, the command stops before starting anything, rather than failing
confusingly ten seconds in.

---

## Build time is not run time

Two separate moments, and it's easy to get caught by the difference.

`NEXT_PUBLIC_*` values are **inlined during `next build`**, not read at
run time. And a build sometimes needs credentials of its own — a Sentry auth
token to upload sourcemaps, a token for a private npm registry.

If your build needs a secret, wrap the build too:

```bash
ward run -- npm run build
```

If it doesn't, don't — a build that holds no credentials is a build that can't
leak any.

---

## Where this works, and where it doesn't

`ward run` starts your process with the secrets in its environment. That means
**you need to control the start command.**

**Works directly:**

| Platform | How |
| --- | --- |
| Docker | `ENTRYPOINT ["ward", "run", "--"]` |
| Fly.io, Render, Railway | Set the start command to `ward run -- ...` |
| ECS, Kubernetes | Same, in the task or pod spec |
| A plain VM or systemd | `ExecStart=/usr/local/bin/ward run -- /srv/app` |
| GitHub Actions, GitLab CI | `run: ward run -- ./deploy.sh` |

**Does not work directly:** Vercel, Netlify Functions, Cloudflare Workers, and
other serverless platforms where the runtime is invoked for you and there is no
launch command to wrap.

On those, the practical approach is to use Warder as the source of truth and
push values into the platform's secret store from CI at deploy time. You keep
one place to rotate and a full audit trail of who changed what — but the
platform then holds plaintext, which is a weaker guarantee than the process
model. Be clear-eyed that it is a trade, not the same thing.

---

## What your process actually receives

Worth knowing, because it explains some behaviour you'd otherwise find
surprising.

**Values go into the environment block, not the argument vector.** They don't
appear in `ps`, and nothing is written to disk. ([`run.go`](../internal/cli/run.go))

**`WARDER_TOKEN` is removed from the child's environment.** Your app receives the
secrets, but not the ability to ask for more of them. This matters more than it
sounds: without it, every dependency in your `node_modules` and every AI agent
running inside that process tree would inherit a credential it could use against
the broker directly.

**Warder's values win over inherited ones.** If your shell already has a stale
`DATABASE_URL`, the delivered value replaces it rather than losing to it.

**The exit code is passed through.** `ward run -- npm test` fails a build
exactly as `npm test` would.

**Signals are forwarded.** Ctrl-C reaches your application and it shuts down
cleanly, instead of the CLI exiting and orphaning it.

**There is no `ward export`.** It would be the obvious convenience command, and
it would write plaintext into a file, a shell history, or a CI log — which is
the exact outcome this product exists to prevent. Secrets go into a process, or
they are revealed to a named person in the dashboard where it is recorded.

---

## Questions people ask

**Does my application code change?**
No. You keep reading `process.env.WHATEVER`. The only change is the command that
starts the process.

**What if Warder is down?**
Your running processes are unaffected — they already have their values in
memory. A process that *restarts* during an outage won't start. Treat the broker
as a dependency of deployment, the same as your container registry.

**Are secrets cached anywhere?**
No. Plaintext is never written to disk or held in a cache, which is why the
deployment has no Redis in it. Each process fetches at startup.

**How do I rotate a database password?**
Rotate it in the dashboard, then restart the applications that use it. You do
not edit any `.env`, any CI configuration, or any deployment manifest.

**Can a developer see the values?**
Only if someone explicitly granted them *Can see* for that environment. No role
grants it — not even Owner. And when they do reveal one, it is recorded against
their name.

**What happens when I revoke a token?**
Immediately, including every short-lived session already issued from it. The
next request is denied, rather than the one after the session would have
expired.

**Can I use one token for several environments?**
No, by design. A token is scoped to one environment. Compromising the staging
token gets an attacker staging.

---

## Where to go next

- [Developer guide](developer-guide.md) — every command, in order
- [Architecture overview](architecture/overview.md) — how it works underneath
- [Limitations](security/limitations.md) — what this does **not** protect
  against, stated plainly. Worth reading before you rely on it.
