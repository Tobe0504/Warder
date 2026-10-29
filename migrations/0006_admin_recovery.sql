-- Recovery links an owner issues for a member.
--
-- The last rung of the ladder. Somebody who has lost their password, their
-- passkeys and their recovery codes has no self-service route back, because
-- Warder sends no email and there is nothing left that proves who they are.
-- An owner can vouch for them instead, out of band, the same way invitations
-- already travel.
--
-- Same shape as an invitation: a minted token, only its verifier stored,
-- single use, expiring. Separate table because the two mean different things
-- and collapsing them would make "who may issue this" one check for two
-- different powers.
CREATE TABLE account_recoveries (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Whose account is being recovered.
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- The organization whose owner vouched, which is what gives them standing
    -- to do this at all. If that membership ends, the link is refused.
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,

    issued_by       uuid        NOT NULL REFERENCES users (id),

    -- SHA-256 of the token. The token itself is shown once, to the issuer, and
    -- is not recoverable from this row.
    token_hash      bytea       NOT NULL UNIQUE,
    token_prefix    text        NOT NULL,

    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    used_at         timestamptz,
    used_ip         inet,
    revoked_at      timestamptz
);

-- At most one live link per account: a second issue supersedes the first
-- rather than leaving two ways in.
CREATE UNIQUE INDEX account_recoveries_one_open_idx
    ON account_recoveries (user_id)
    WHERE used_at IS NULL AND revoked_at IS NULL;

CREATE INDEX account_recoveries_org_idx ON account_recoveries (organization_id);
