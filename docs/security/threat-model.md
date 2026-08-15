# Threat model

This document states what Warder protects, from whom, and — as precisely as
possible — what it does not protect. The last part matters most: a security
document that only lists strengths is a marketing document.

## What is being protected

| Asset | Where it lives | Consequence of compromise |
|---|---|---|
| Secret plaintext | Transiently in memory during encryption, decryption, and delivery | Direct compromise of whatever the credential opens |
| Key encryption keys | Operator-supplied, in process memory; a KMS in production | Every ciphertext in the database becomes readable |
| Data encryption keys | One per secret version, wrapped at rest, unwrapped transiently | One version of one secret |
| Machine tokens | Held by workloads; a SHA-256 verifier in the database | Whatever that token's scope permits, until revoked |
| Browser and CLI sessions | HttpOnly cookie; a file at mode 0600 | That person's authority, until revoked |
| The service credential | The BFF's server-side environment | Ability to reach the core API as any session-bearer |
| The audit trail | `audit_events`, append-only | Loss of the record of what happened |
| Secret metadata | `public` schema | Discloses which credentials exist and who uses them |

## Actors

Each is treated as potentially hostile, which is different from assuming each is
hostile. The design question is what a given actor can reach *if* they turn out
to be.

- **Anonymous network attacker.** Can reach whatever is exposed.
- **Authenticated developer.** Legitimate access to some environments.
- **AI coding agent.** Runs with a developer's tooling on a developer's machine.
  Explicitly not trusted; see below.
- **Contractor.** Time-bounded legitimate access.
- **Compromised workload.** A container or CI job an attacker now controls.
- **Malicious insider with administrative rights.** Holds `MANAGE_ACCESS`.
- **Database reader.** Has a dump, a replica, or a backup.
- **Infrastructure operator.** Can read process memory and environment.

## Trust boundaries

```
  Browser                          untrusted
     │  HTTPS, HttpOnly + SameSite=strict cookie, CSRF token, CSP
     ▼
  Next.js BFF                      trusted application boundary
     │  private network, service credential, fixed paths only
     ▼
  Core API                         authorization boundary
     │
     ├── policy engine             every decision, deny by default
     ├── encryption service        the only place plaintext meets a cipher
     │
     ▼
  PostgreSQL                       encrypted storage boundary
     │   public.*           metadata
     │   secret_material.*  ciphertext, separately grantable
     ▼
  Key provider                     key custody boundary — outside the database

  Runtime (separate listener)      authorized secret consumer
```

The boundary that does the most work is the one between the browser and the core
API. The browser can never reach the core API, because every route on that
surface requires a service credential that exists only on the BFF's server side.
A stolen session cookie is not sufficient.

---

## Threats and what is done about them

### Database compromise

**Threat.** An attacker obtains a dump, a replica, or a backup.

**Mitigation.** Secret values are encrypted with AES-256-GCM under a per-version
data key, which is itself wrapped by a key encryption key that is never stored in
the database and never in the source tree. A complete dump yields ciphertext and
metadata, not credentials. Passwords are Argon2id. Tokens and sessions are stored
as SHA-256 verifiers and cannot be replayed from a dump.

**Also mitigated.** An attacker with database *write* access cannot relocate
ciphertext. Every value is bound by AEAD additional authenticated data to its
organization, project, environment, secret, and version. Copying a production
row into a development secret produces a decryption failure, not a disclosure.
This is tested in `internal/crypto/envelope_test.go`.

**Residual risk.** An attacker holding both the database and the key material
reads everything. Key custody is the control that matters, which is why
production should use a KMS where the key cannot be exported at all.

### Token theft

**Threat.** A machine token leaks through a log, an image layer, a CI variable,
or a compromised container.

**Mitigation.** Tokens are scoped to exactly one project and one environment, so
a leaked development token cannot reach production regardless of what its
identity is granted — scope is checked before grants are consulted. Tokens can be
narrowed further to specific keys. They are revocable, and revoking one also
revokes every short-lived session derived from it, so revocation takes effect on
the next request rather than after a delay. The long-lived token is presented
once per process start; the credential that accompanies actual secret retrieval
lives five minutes.

**Residual risk.** Until revoked, a stolen token can be used from anywhere. The
MVP has no source-address binding and no proof-of-possession. This is the main
reason the identity layer is an interface: workload identity, OIDC, and
Kubernetes service accounts each remove the stored bearer token entirely.

### Session theft

**Threat.** A session cookie is captured.

**Mitigation.** `HttpOnly` puts it out of reach of client script. `Secure` keeps
it off plain HTTP. `SameSite=strict` means it is not attached to any cross-site
request. Sessions are server-side records that can be revoked immediately, and
removing a member revokes all of theirs. Browser sessions and CLI logins are
distinct kinds and each is refused on the other's surface, so a stolen browser
session cannot be used to retrieve secrets from the runtime API.

**Residual risk.** An attacker with a live session can act as that user until it
is revoked or expires.

### Cross-site scripting

**Threat.** Injected script runs in the dashboard.

**Mitigation.** React escapes all interpolated values, and
`dangerouslySetInnerHTML` appears nowhere in the codebase — enforced by
`web/scripts/check-boundaries.mjs`, which runs in CI. A nonce-based Content
Security Policy without `unsafe-inline` for scripts limits what injected markup
could execute. `connect-src 'self'` blocks exfiltration to a third-party
endpoint. The session cookie is `HttpOnly`, so script cannot read it. Secret
values are not present in any page — the only way to obtain one in the browser
is an explicit reveal, which requires `READ_SECRET` and is audited.

The same check fails the build on any `NEXT_PUBLIC_` variable, on a client
component that transitively reaches a server-only module, and on any use of
`localStorage`, `sessionStorage`, or `indexedDB` — so a session or a revealed
value cannot come to rest somewhere script can read it later.

The development server relaxes the policy with `'unsafe-eval'`, because Next.js
compiles modules with `eval` for hot reloading and a strict policy leaves the
whole dashboard non-interactive. The relaxation is keyed on `NODE_ENV`, which is
`production` in any built artifact. This is worth stating plainly: a security
control that makes local development impossible is a control that gets commented
out, and the commented-out version is what ships.

**Residual risk.** Script running in the page can still act as the user for as
long as it runs, including calling reveal if that user holds `READ_SECRET`. This
is a substantial part of why `READ_SECRET` is never granted by role.

### Cross-site request forgery

**Threat.** Another site causes a state-changing request.

**Mitigation.** Three independent controls: `SameSite=strict` (the cookie is not
sent at all), an `Origin` check (a page cannot forge it), and a double-submit
token (only same-origin script can read the cookie holding it). CORS is
deliberately not counted as one of these, because a simple request is delivered
before CORS is consulted.

**Including sign-in.** Sign-in and organization creation have no session yet and
therefore no token to check, but they still enforce the origin check. Without
it, another site can forge a sign-in carrying the *attacker's* credentials — no
cookie needed, since the victim has no session — and leave the browser holding a
valid session for the attacker's organization. The victim then works inside it,
and any secret they add lands somewhere the attacker can read. It is a quiet
attack, because everything appears to work.

### Configuration error

**Threat.** The dashboard is deployed with a credential that does not match its
endpoint, or with transport security silently disabled.

**Mitigation.** The BFF takes one connection URI rather than four independent
variables, so a partially-updated configuration is not expressible. It either
parses completely or the process refuses to start. The deployment posture is
part of the URI, so `warder+insecure://` (plain HTTP) with `/production` is a
startup failure rather than a downgrade nobody notices, and production requires
an explicit browser origin so the cross-site checks above have something to
compare against.

The credential now lives inside a URL, which is the most commonly logged kind of
string there is. That is handled explicitly: the parsed connection exposes a
`redacted` form for anything that describes it, and the log redactor scrubs both
`scheme://user:pass@` and the bare `scheme://credential@` shape this format
uses.

### Server-side request forgery

**Threat.** An attacker induces a server to fetch an address of their choosing —
cloud metadata, an internal service.

**Mitigation.** There is no generic proxy endpoint and no path where a
caller-supplied URL is fetched. Every core API call is built from a literal path
at the call site and validated to be relative; identifiers taken from URL
segments are checked against a UUID pattern before being interpolated. Redirects
are not followed on any client, so an upstream cannot redirect a request carrying
the service credential elsewhere. Warder makes no outbound requests to
user-supplied destinations at all — which is the strongest available answer,
since it means there is no allowlist to get wrong.

### Privilege escalation

**Threat.** An identity acquires authority it was not granted.

**Mitigation.** Authorization is one function, `authz.Engine.Authorize`, and
every handler goes through one gate that calls it. Capabilities never imply one
another: `USE_SECRET` does not confer `READ_SECRET`. Credential scope is checked
before grants, so a token cannot widen its identity. The `/runtime/auth` exchange
copies the presented credential's ceiling and cannot exceed it; for a human's CLI
login it issues `USE_SECRET` only, so a runtime session can never reveal a value.
Grant creation requires each narrowing level to be named explicitly, so an
omitted form field produces a narrow grant rather than a wildcard.

**Residual risk.** `MANAGE_ACCESS` is genuinely powerful: a holder can grant
themselves `READ_SECRET`. This is an accepted property, not a gap. The mitigation
is that doing so is a distinct, audited act flagged in the trail with
`grants_plaintext_visibility` and `self_granted`, rather than an ambient property
of being an administrator.

### Insider threat

**Threat.** Someone with legitimate access abuses it.

**Mitigation.** No role confers plaintext visibility. Reading a value requires an
explicit grant, which is recorded with a stated reason and normally an expiry.
Both the reveal request and the disclosure are audited. The audit trail is
append-only, enforced by a database trigger and by withholding `UPDATE` and
`DELETE` from the application's database role, so an insider cannot erase the
record of what they read.

**Residual risk.** An administrator can still read what they choose to. The
system makes that visible and attributable; it does not make it impossible.

### Log leakage

**Threat.** A secret reaches logs, telemetry, or an error report.

**Mitigation.** Two layers. Plaintext travels in `secretvalue.Value`, which
renders as `[redacted]` through every `fmt` verb, through `slog`, and refuses to
marshal to JSON at all — an accidental include in a response fails the request
rather than succeeding quietly. Behind that, the application logger redacts by
attribute name and scans every logged string for credential shapes, including
Warder's own token format and common vendor formats. Extracting plaintext
requires calling `.Expose()`, which makes every such point greppable:

```bash
git grep -n '\.Expose\|\.ExposeString'
```

The result should be a short, reviewable list. Tested in
`internal/apitest/security_leak_test.go`.

### Browser compromise

**Threat.** The user's browser or an extension is hostile.

**Mitigation.** Values are not present in the dashboard. Metadata is masked with
a constant rather than a length-preserving mask, so a listing does not disclose
credential lengths. A revealed value is held in one component-local variable and
clears itself after 30 seconds.

**Residual risk.** A compromised browser sees whatever the user sees. Nothing
here changes that.

### Malicious dependency

**Threat.** A compromised package exfiltrates secrets.

**Mitigation.** The Go dependency set is deliberately small — `pgx`,
`google/uuid`, `golang.org/x/crypto`, `golang.org/x/term` — and the HTTP router,
validation, and migration runner are standard library or first-party code, so
there is less surface to compromise. Lockfiles are committed, `go.sum` verifies
module checksums, and CI runs `govulncheck` and `npm audit`.

**Residual risk.** The frontend's dependency tree is larger by an order of
magnitude. It handles no plaintext except during an explicit reveal, but it does
handle sessions.

### Runtime compromise

**Threat.** An attacker controls a process that has been given secrets.

**Mitigation.** Blast radius is bounded by scope: one project, one environment,
optionally specific keys. `ward run` strips `WARDER_TOKEN` from the child's
environment, so a compromised process holds the secrets it was given but not the
credential to request more.

**Residual risk.** The process has those plaintext values, and they are valid
until rotated at the provider. This is unavoidable — see
[limitations](./limitations.md).

### AI agent prompt injection

**Threat.** Content an agent reads — an issue, a web page, a dependency's README —
instructs it to exfiltrate credentials, and the agent complies.

**Mitigation.** This threat is the reason for much of the design, so it gets its
own treatment below.

### Credential exfiltration

**Threat.** Someone with legitimate use of a credential copies it out.

**Mitigation.** For a human: they never receive it, unless granted `READ_SECRET`.
For a process: it does receive it, and the mitigation is limiting which process,
which environment, and which keys, plus recording every use.

### Replay attacks

**Threat.** A captured request is replayed.

**Mitigation.** TLS provides replay protection in transit. Runtime sessions
expire in minutes and are revocable. Encryption is nonce-based AEAD with a fresh
data key per version, so ciphertext is never reused across contexts.

**Residual risk.** A captured bearer token is replayable until it expires or is
revoked. Proof-of-possession would fix this and is not in the MVP.

### Brute-force token attacks

**Threat.** An attacker guesses a token.

**Mitigation.** Tokens carry 256 bits of entropy from the operating system's
CSPRNG. Guessing is not a realistic attack. Rate limiting exists as defence in
depth and to bound resource consumption, not because the entropy is in doubt.
Passwords are the case where the limit actually matters, and login is limited to
five attempts per minute per address on top of Argon2id.

**Residual risk.** The MVP rate limiter is per-process. Behind N instances the
effective limit is N times the configured one, and a restart clears it. For the
login limit specifically, a production deployment should move this to a shared
backend.

---

## The AI agent, stated explicitly

An AI coding agent is not a developer with a keyboard. It is a program that
reads untrusted input — issues, pull requests, documentation, dependency READMEs,
web pages — and can be induced by that input to take actions its operator did not
intend. It also inspects files, runs commands, reads environment variables, makes
network requests, and produces transcripts that are often stored and sometimes
shared.

**Warder therefore does not treat an agent as its developer.** An agent gets its
own identity, its own grants, and its own scoped credential. Concretely:

- An agent identity can be created with `expiresAt`, so an agent session is
  temporary by construction rather than by someone remembering to clean it up.
- A token issued to it names one project and one environment, and may name
  specific keys.
- It holds `USE_SECRET` and not `READ_SECRET`, so `ward run -- npm test`
  succeeds and nothing it can do prints a credential.
- The admin API refuses its credential outright, so it cannot grant itself
  anything, mint itself a token, rotate a secret, or read the audit trail.

What this does **not** do: an agent that runs `ward run -- npm test` starts a
process whose environment contains the test credentials, and the agent can read
that process's environment. The protection is that those are test credentials in
a development environment, scoped as narrowly as the operator chose — not that
the agent is prevented from reading what it was authorized to use.

The corresponding operational advice: give agents development scopes, name the
keys they need, set an expiry, and never grant an agent identity `READ_SECRET`.

Tested in `internal/apitest/security_agent_test.go`.

---

## Known gaps in this MVP

Listed plainly because an undocumented gap is worse than a documented one.

1. **Rate limiting is per-process.** Multi-instance deployments multiply every
   limit; a restart clears the buckets.
2. **Organization creation is unauthenticated.** Anyone who can reach the API can
   create a tenant. A real deployment must gate this.
3. **No multi-factor authentication.** Password plus session only.
4. **No re-authentication before reveal.** A live session with `READ_SECRET` can
   reveal without re-entering a password. The grant is expected to carry the
   time bound instead.
5. **Cloud KMS providers are interfaces, not implementations.** The local keyring
   holds key material in process memory and logs no key access.
6. **No automatic upstream rotation.** Rotation stores a new value; it does not
   change the credential at the provider. The API and the interface both say so.
7. **Machine tokens are bearer credentials.** No source-address binding, no
   proof-of-possession.
8. **No alerting.** Audit events are recorded but nothing watches them. A burst
   of denials or an unusual reveal should page someone; that is not built.
9. **The runtime session table grows** between purges, bounded by a background
   sweep every fifteen minutes.

## What Warder does not claim

- Not zero-knowledge. The server decrypts values; it necessarily can read them.
- Not "impossible to exfiltrate". A process authorized to use a credential holds
  that credential in its memory.
- Not "developers can never access secrets". An administrator can grant
  `READ_SECRET`, including to themselves.
- Not certified against any standard. It is an MVP.

The accurate claim is narrower and still worth something:

> Developers, agents, and CI systems can use the credentials their software
> needs without being granted plaintext visibility, and every exception to that
> is explicit, time-bounded, and recorded.
