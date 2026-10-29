-- Challenges for a sign-in that names no account.
--
-- A discoverable passkey carries the account handle inside it, so the
-- ceremony begins before anybody knows whose account it is. The challenge
-- therefore cannot hang off a user row the way the others do, and gets its own
-- table keyed by an opaque handle that identifies the challenge and nothing
-- else.
CREATE TABLE webauthn_signin_challenges (
    handle     uuid        PRIMARY KEY,
    session    jsonb       NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Abandoned ceremonies are the common case: somebody opens the sign-in page
-- and walks away. Expiry is what keeps this table from growing forever.
CREATE INDEX webauthn_signin_challenges_expiry_idx
    ON webauthn_signin_challenges (expires_at);
