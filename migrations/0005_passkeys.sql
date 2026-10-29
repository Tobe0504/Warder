-- Passkeys, as a second factor.
--
-- Two tables and no new column on users: WebAuthn needs a stable, opaque
-- handle for a person, and the user's uuid already is one. Using the email
-- would put an identifier in the authenticator that changes when somebody
-- changes their address.
CREATE TABLE webauthn_credentials (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- The authenticator's own handle for this credential. Unique across every
    -- account: one physical key registered twice would otherwise be two rows
    -- the sign counter could not tell apart.
    credential_id    bytea       NOT NULL UNIQUE,

    -- The COSE public key. Public by construction; the private half never
    -- leaves the authenticator, which is the whole point of the scheme.
    public_key       bytea       NOT NULL,

    attestation_type text        NOT NULL DEFAULT 'none',
    transports       text[]      NOT NULL DEFAULT '{}',
    aaguid           bytea,

    -- Replay detection. An authenticator that counts increments this on every
    -- assertion; a value that fails to advance means the assertion was
    -- replayed or the credential was cloned.
    sign_count       bigint      NOT NULL DEFAULT 0,

    backup_eligible  boolean     NOT NULL DEFAULT false,
    backup_state     boolean     NOT NULL DEFAULT false,

    -- What the person called it, so a list of three keys is readable.
    nickname         text        NOT NULL,

    created_at       timestamptz NOT NULL DEFAULT now(),
    last_used_at     timestamptz
);

CREATE INDEX webauthn_credentials_user_idx ON webauthn_credentials (user_id);

-- The challenge issued at the start of a ceremony, held until it completes.
--
-- Server-side rather than in a cookie, because a challenge the client can
-- rewrite is not a challenge. Single use: consuming one deletes it, so an
-- assertion cannot be replayed against the challenge that produced it.
CREATE TABLE webauthn_challenges (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    text        NOT NULL CHECK (purpose IN ('REGISTER', 'LOGIN')),
    session    jsonb       NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX webauthn_challenges_user_idx ON webauthn_challenges (user_id, purpose);

-- A password that has been accepted, waiting on its second factor.
--
-- Deliberately not a row in user_sessions with a "pending" flag. A half-made
-- session living in the same table as real ones is one forgotten WHERE clause
-- away from being a real one, and nothing in the system looks this table up
-- except the two endpoints that finish the ceremony.
CREATE TABLE mfa_pending (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea       NOT NULL UNIQUE,

    -- The session kind to mint once the factor is satisfied, so that a CLI
    -- login does not come back as a browser session.
    kind       text        NOT NULL CHECK (kind IN ('BROWSER', 'CLI')),

    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
