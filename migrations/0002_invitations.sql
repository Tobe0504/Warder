-- Membership invitations.
--
-- Replaces the original "add a member" flow, in which the owner chose the new
-- member's password and then had to send it to them. That put a working
-- credential in a Slack message and left the owner permanently knowing how to
-- sign in as someone else — a poor shape for a product whose argument is that
-- people should not hold credentials they do not need.
--
-- An invitation carries the decision (which organization, which role, when the
-- membership should lapse) but no credential. The invitee sets their own
-- password when they accept, and the owner never learns it.

CREATE TABLE membership_invitations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,

    -- Both are fixed at invite time and are never read from the acceptance
    -- request. Whoever holds the token can choose their password and their
    -- display name; they cannot choose which address they become, and they
    -- cannot promote themselves.
    email           text        NOT NULL,
    name            text        NOT NULL,
    role            text        NOT NULL CHECK (role IN ('OWNER', 'ADMIN', 'DEVELOPER', 'VIEWER')),

    -- Bounds the membership that acceptance creates, not the invitation. This
    -- is the contractor workflow: set a date now, and their access ends on its
    -- own later without anyone having to remember.
    membership_expires_at timestamptz,

    -- The same public-handle-plus-verifier shape as every other credential in
    -- the system: lookup by the public half, compare the secret half against a
    -- SHA-256 verifier. The token itself is never stored.
    public_id       text        NOT NULL UNIQUE,
    token_hash      bytea       NOT NULL,

    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid        REFERENCES users (id),

    -- Invitations are short-lived on purpose. An invite link forwarded into a
    -- group chat and forgotten is a way into the organization, so it stops
    -- working on its own.
    expires_at      timestamptz NOT NULL,

    -- Single use. Acceptance sets these in the same transaction that creates
    -- the membership, conditional on accepted_at being null, so two
    -- simultaneous redemptions cannot both succeed.
    accepted_at     timestamptz,
    accepted_user_id uuid       REFERENCES users (id),

    revoked_at      timestamptz
);

CREATE INDEX membership_invitations_org_idx
    ON membership_invitations (organization_id, created_at DESC);

-- At most one live invitation per address per organization. Without this,
-- inviting the same person twice leaves two valid tokens and revoking the one
-- you can see does not close the door.
CREATE UNIQUE INDEX membership_invitations_pending_key
    ON membership_invitations (organization_id, lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
