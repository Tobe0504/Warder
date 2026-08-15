# Limitations

What Warder does not do, stated plainly. A security product that only documents
its strengths teaches its users to trust it in situations where they should not.

## The process limit

Once a plaintext value is in a process's environment, that process can read it.
It can print it, write it to a file, or send it anywhere it can reach. Warder
delivers a credential to a process; it does not follow it afterwards.

This is not a gap that a future version closes. It is a property of the problem:
a program that uses a database password must have the database password. Secure
enclaves, sealed file descriptors, and in-memory-only delivery each narrow the
window, and none of them remove it.

What Warder actually changes is who needs to hold the credential in order for the
software to run:

| | Before | With Warder |
|---|---|---|
| Developer | Has it in `.env` | Never sees it |
| Agent | Reads the developer's `.env` | Uses a scoped credential it cannot print |
| CI | Holds it as a build variable | Authenticates and receives only its scope |
| The process | Has it | Has it |
| Departing contractor | Has copies | Loses access without rotation |

The last row is where the real operational value sits. Ending someone's access
is a grant change, not a rotation of every credential they might have seen.

## What the CLI can and cannot promise

`ward run` does not print values, does not write them to disk, does not put them
in argument vectors, and removes `WARDER_TOKEN` from the child's environment so a
compromised process cannot request more than it was given.

It cannot prevent the program it starts from doing any of those things. If your
application logs `process.env` on startup, Warder will have delivered a secret
into a process that then logs it.

## Security controls not implemented

| Gap | Consequence | Note |
|---|---|---|
| Rate limiting is per-process | Effective limits multiply across instances; restarts reset them | Matters most for login |
| Open organization creation | Anyone reachable can create a tenant | Must be gated before real use |
| No multi-factor authentication | Password compromise is account compromise | |
| No re-authentication before reveal | A live session with `READ_SECRET` reveals without a password prompt | Time-bounded grants are the intended control |
| Cloud KMS not implemented | Local keyring holds keys in process memory, logs no key access | Interfaces exist and are exercised by the local provider |
| No automatic upstream rotation | Rotating in Warder does not change the credential at the provider | The API and interface both say so explicitly |
| Bearer machine tokens | A stolen token works from anywhere until revoked | Workload identity is the fix; the interface is ready for it |
| No alerting | Denials and reveals are recorded, and nothing watches them | |
| Single organization per user | A person cannot belong to two tenants | Schema supports it; resolution picks the first |

## Cryptographic notes

- **AES-256-GCM with random 96-bit nonces.** Each secret version gets a fresh
  data key used for exactly one encryption, so nonce reuse on values is
  impossible. The key encryption key wraps many data keys, where the birthday
  bound is roughly 2³² operations per key, far beyond realistic volumes, and the
  reason key versioning exists.
- **Encryption context is mandatory.** An all-zero context is rejected rather
  than tolerated, so ciphertext cannot accidentally be written unbound.
- **SHA-256 for tokens, Argon2id for passwords.** Deliberate: a 256-bit random
  token has no guessing attack to slow down, and putting a memory-hard function
  on the secret-retrieval path would cost latency for nothing. Passwords are
  human-chosen and get the memory-hard treatment.
- **`Zeroize` is best effort.** Go's garbage collector may already have copied a
  buffer, values may be spilled to the stack, and pages may have been swapped.
  It shortens a window; it does not close one.

## Operational gaps

- No backup or restore procedure is automated. Recovering a Warder deployment
  means recovering two things kept deliberately apart: the database and the
  keyring, and neither is any use without the other.
- No key rotation job. The schema records a key version per row and the
  encryption layer decrypts under old versions, so re-encryption is possible:
  but nothing performs it.
- Audit retention is unbounded. Trimming requires the privileged procedure in
  `deploy/sql/erase-organization.sql`, by design.
- No metrics or health checks beyond liveness.

## Compliance

Warder has not been audited, certified, or assessed against SOC 2, ISO 27001,
PCI DSS, HIPAA, or any other framework. Several of its design choices, envelope
encryption, an append-only audit trail, least-privilege database roles: are the
kind of thing such assessments look for. That is not the same as having passed
one, and this MVP should not be described as compliant with anything.
