-- Recovery codes.
--
-- Warder sends no email, by design: invitations are links an administrator
-- copies and hands over. That leaves the usual "click this link to reset your
-- password" route unavailable, so a person who forgets their password needs
-- something they already hold. These are it.
--
-- Stored as SHA-256 of the code, not as an Argon2 hash. Argon2 exists to make
-- guessing a low-entropy human password expensive; these are generated with 75
-- bits of entropy and never chosen by a person, so a slow KDF would buy
-- nothing and cost a second per verification. Same reasoning, and the same
-- treatment, as the session tokens in user_sessions.
CREATE TABLE recovery_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- SHA-256 of the normalized code. A database reader cannot reconstruct a
    -- code from this column, and the lookup is a single indexed probe rather
    -- than a scan comparing every row.
    code_hash  bytea       NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    -- Set the moment a code is spent. A code is single use, and the row is
    -- kept rather than deleted so that "this account was recovered, here is
    -- when and from where" survives the event.
    used_at    timestamptz,
    used_ip    inet,

    -- Two codes for one person can never collide, which makes consumption a
    -- single conditional update with no ambiguity about which row was meant.
    UNIQUE (user_id, code_hash)
);

-- Counting what is left, and finding the one being spent, both only ever look
-- at unused codes.
CREATE INDEX recovery_codes_unused_idx
    ON recovery_codes (user_id)
    WHERE used_at IS NULL;
