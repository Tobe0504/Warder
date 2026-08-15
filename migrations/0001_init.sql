-- Warder initial schema.
--
-- Two schemas are used deliberately:
--
--   public           metadata: who exists, what secrets exist, who may use them
--   secret_material  ciphertext and wrapped data keys, nothing else
--
-- The split exists so that a database role can be granted the whole metadata
-- surface — reporting, dashboards, support tooling, an analytics replica —
-- while holding no privilege at all on secret_material. Everything an operator
-- normally needs to look at lives in public. See deploy/sql/roles.sql.

CREATE SCHEMA IF NOT EXISTS secret_material;


-- ---------------------------------------------------------------------------
-- Tenancy and humans
-- ---------------------------------------------------------------------------

CREATE TABLE organizations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL,
    slug        text        NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text        NOT NULL,
    name          text        NOT NULL,
    -- Argon2id PHC string. Never selected outside the authentication path.
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    disabled_at   timestamptz
);

-- Addresses are compared case-insensitively so that a second account cannot be
-- registered by varying capitalization of an existing one.
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

-- Membership carries an expiry, which is what makes the contractor workflow
-- work without rotating credentials: when the row stops being active the
-- person's authority evaporates, and the secrets themselves are untouched.
CREATE TABLE memberships (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            text        NOT NULL CHECK (role IN ('OWNER', 'ADMIN', 'DEVELOPER', 'VIEWER')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid        REFERENCES users (id),
    expires_at      timestamptz,
    revoked_at      timestamptz,
    UNIQUE (organization_id, user_id)
);

CREATE INDEX memberships_user_idx ON memberships (user_id);


-- ---------------------------------------------------------------------------
-- Browser and CLI sessions
--
-- Sessions are opaque random values verified against a stored SHA-256 hash.
-- They are not JWTs: revocation has to be immediate and unconditional, because
-- "what happens if I revoke access?" must answer "the next request is denied",
-- not "within the token's remaining lifetime".
-- ---------------------------------------------------------------------------

CREATE TABLE user_sessions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id uuid        REFERENCES organizations (id) ON DELETE CASCADE,
    kind            text        NOT NULL CHECK (kind IN ('BROWSER', 'CLI')),
    -- SHA-256 of the presented token. A database reader cannot replay a
    -- session from this column.
    token_hash      bytea       NOT NULL UNIQUE,
    -- Non-secret leading characters, so a person can recognize their own
    -- session in a list without the value being reconstructable.
    token_prefix    text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz,
    last_used_at    timestamptz,
    ip_address      inet,
    user_agent      text
);

CREATE INDEX user_sessions_user_idx ON user_sessions (user_id);
CREATE INDEX user_sessions_expiry_idx ON user_sessions (expires_at);


-- ---------------------------------------------------------------------------
-- Projects and environments
-- ---------------------------------------------------------------------------

CREATE TABLE projects (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    slug            text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

-- Environments carry no privilege ranking in the schema or in the policy
-- engine. "production" is not a keyword anywhere in the authorization path;
-- isolation comes from grants and token scopes naming a specific environment
-- id. That is what makes a custom "preview" environment exactly as isolated as
-- the built-in ones.
CREATE TABLE environments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    slug        text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);


-- ---------------------------------------------------------------------------
-- Secrets
--
-- This table is metadata only and has no value column by construction. A dump
-- of public.secrets tells you which credentials exist, never what they are.
-- ---------------------------------------------------------------------------

CREATE TABLE secrets (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    key            text        NOT NULL,
    description    text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    created_by     uuid        REFERENCES users (id),
    deleted_at     timestamptz,
    -- Answers "is anything still using this?" before an administrator revokes.
    last_used_at   timestamptz
);

CREATE UNIQUE INDEX secrets_environment_key_key
    ON secrets (environment_id, key) WHERE deleted_at IS NULL;

CREATE TABLE secret_versions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    secret_id          uuid        NOT NULL REFERENCES secrets (id) ON DELETE CASCADE,
    version            integer     NOT NULL CHECK (version > 0),
    status             text        NOT NULL CHECK (status IN ('ACTIVE', 'SUPERSEDED', 'REVOKED')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    created_by         uuid        REFERENCES users (id),
    expires_at         timestamptz,
    revoked_at         timestamptz,
    -- Which key encryption key version wrapped this version's data key.
    -- Recorded per row so key rotation is incremental rather than a
    -- stop-the-world rewrite of every secret.
    encryption_key_id  text        NOT NULL,
    UNIQUE (secret_id, version)
);

-- At most one active version per secret, enforced by the database rather than
-- by application discipline: two active versions would make "which value does
-- my application get" ambiguous.
CREATE UNIQUE INDEX secret_versions_one_active
    ON secret_versions (secret_id) WHERE status = 'ACTIVE';

CREATE INDEX secret_versions_secret_idx ON secret_versions (secret_id, version DESC);

-- The ciphertext itself, in its own schema. The row is keyed by the version it
-- belongs to and holds no business metadata, so a reader of this table learns
-- nothing about which credential a blob represents without also holding read
-- access on public.
CREATE TABLE secret_material.secret_version_material (
    secret_version_id uuid PRIMARY KEY REFERENCES public.secret_versions (id) ON DELETE CASCADE,
    scheme            integer NOT NULL,
    algorithm         text    NOT NULL,
    -- Key encryption key version. The key itself is never stored here, or
    -- anywhere else in this database.
    key_id            text    NOT NULL,
    wrapped_data_key  bytea   NOT NULL,
    nonce             bytea   NOT NULL,
    ciphertext        bytea   NOT NULL
);


-- ---------------------------------------------------------------------------
-- Machine identities and tokens
--
-- A machine identity is a first-class subject: an application, a CI pipeline,
-- or an AI coding agent session. It never inherits the authority of the human
-- who created it.
-- ---------------------------------------------------------------------------

CREATE TABLE machine_identities (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    actor_type      text        NOT NULL CHECK (actor_type IN ('SERVICE', 'AI_AGENT', 'CI', 'WORKLOAD')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid        REFERENCES users (id),
    disabled_at     timestamptz,
    -- Bounds the identity itself, which is how an agent session is made
    -- inherently temporary rather than temporary by convention.
    expires_at      timestamptz,
    UNIQUE (organization_id, name)
);

CREATE TABLE machine_tokens (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_identity_id uuid        NOT NULL REFERENCES machine_identities (id) ON DELETE CASCADE,
    organization_id     uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                text        NOT NULL,

    -- Scope is mandatory, not optional. A token always names exactly one
    -- project and one environment, so a development token is structurally
    -- incapable of addressing production no matter what its identity holds.
    project_id          uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    environment_id      uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,

    -- The capability ceiling of this credential. Intersected with the
    -- identity's grants at evaluation time; it can only narrow, never widen.
    capabilities        text[]      NOT NULL CHECK (cardinality(capabilities) > 0),
    -- Optional further narrowing to specific keys, for least privilege.
    secret_keys         text[]      NOT NULL DEFAULT '{}',

    token_hash          bytea       NOT NULL UNIQUE,
    token_prefix        text        NOT NULL,

    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid        REFERENCES users (id),
    expires_at          timestamptz,
    revoked_at          timestamptz,
    last_used_at        timestamptz
);

CREATE INDEX machine_tokens_identity_idx ON machine_tokens (machine_identity_id);
CREATE INDEX machine_tokens_project_idx ON machine_tokens (project_id);
CREATE INDEX machine_tokens_prefix_idx ON machine_tokens (token_prefix);

-- Short-lived credentials minted by POST /runtime/auth and presented to
-- POST /runtime/secrets. The long-lived token is used once per process start;
-- the credential that actually accompanies secret retrieval lives minutes.
CREATE TABLE runtime_sessions (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id      uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    subject_type         text        NOT NULL CHECK (subject_type IN ('USER', 'MACHINE')),
    subject_id           uuid        NOT NULL,
    actor_type           text        NOT NULL,
    project_id           uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    environment_id       uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    capabilities         text[]      NOT NULL,
    secret_keys          text[]      NOT NULL DEFAULT '{}',
    -- The credential this session was minted from, so revoking a machine token
    -- can also invalidate sessions derived from it.
    source_credential_id uuid,
    token_hash           bytea       NOT NULL UNIQUE,
    token_prefix         text        NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    expires_at           timestamptz NOT NULL,
    revoked_at           timestamptz
);

CREATE INDEX runtime_sessions_source_idx ON runtime_sessions (source_credential_id);
CREATE INDEX runtime_sessions_expiry_idx ON runtime_sessions (expires_at);


-- ---------------------------------------------------------------------------
-- Access grants
--
-- The explicit statement that an identity may exercise capabilities over part
-- of the secret tree. USE_SECRET and READ_SECRET are only ever conferred here,
-- never by a role, so the answer to "who can see this value" is a query
-- against one table rather than an inference across several.
-- ---------------------------------------------------------------------------

CREATE TABLE access_grants (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,

    subject_type    text        NOT NULL CHECK (subject_type IN ('USER', 'MACHINE')),
    subject_id      uuid        NOT NULL,

    -- Narrowing is left to right. NULL means "everything at this level", and
    -- the API requires callers to opt into each wildcard explicitly so that an
    -- omitted form field can never silently widen a grant.
    project_id      uuid        REFERENCES projects (id) ON DELETE CASCADE,
    environment_id  uuid        REFERENCES environments (id) ON DELETE CASCADE,
    secret_id       uuid        REFERENCES secrets (id) ON DELETE CASCADE,

    capabilities    text[]      NOT NULL CHECK (cardinality(capabilities) > 0),

    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid        REFERENCES users (id),
    expires_at      timestamptz,
    revoked_at      timestamptz,
    -- Why this grant exists. For READ_SECRET the auditor's question is rarely
    -- "who can see this" and almost always "why can they".
    reason          text        NOT NULL DEFAULT '',

    -- A narrower level cannot be specified without the levels above it, which
    -- would produce a grant whose meaning is ambiguous.
    CHECK (environment_id IS NULL OR project_id IS NOT NULL),
    CHECK (secret_id IS NULL OR environment_id IS NOT NULL)
);

CREATE INDEX access_grants_subject_idx ON access_grants (organization_id, subject_type, subject_id);
CREATE INDEX access_grants_project_idx ON access_grants (project_id);
CREATE INDEX access_grants_environment_idx ON access_grants (environment_id);
CREATE INDEX access_grants_secret_idx ON access_grants (secret_id);


-- ---------------------------------------------------------------------------
-- Audit
-- ---------------------------------------------------------------------------

CREATE TABLE audit_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    occurred_at     timestamptz NOT NULL DEFAULT now(),

    event_type      text        NOT NULL,
    outcome         text        NOT NULL CHECK (outcome IN ('SUCCESS', 'DENIED', 'FAILURE')),

    actor_type      text        NOT NULL,
    actor_id        uuid,
    -- Denormalized display name, so the log stays readable after the actor is
    -- deleted. An audit trail that decays into unresolvable ids is not one.
    actor_label     text        NOT NULL DEFAULT '',
    credential_id   uuid,

    project_id      uuid,
    environment_id  uuid,
    secret_id       uuid,
    secret_key      text,
    token_id        uuid,

    ip_address      inet,
    user_agent      text,
    -- Why a request was denied, in terms safe to show an administrator.
    reason          text        NOT NULL DEFAULT '',

    -- Structured detail. Writes go through the audit package, which strips
    -- anything resembling secret material before it reaches this column.
    metadata        jsonb       NOT NULL DEFAULT '{}'
);

CREATE INDEX audit_events_org_time_idx ON audit_events (organization_id, occurred_at DESC);
CREATE INDEX audit_events_project_idx ON audit_events (project_id, occurred_at DESC);
CREATE INDEX audit_events_secret_idx ON audit_events (secret_id, occurred_at DESC);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_id, occurred_at DESC);

-- The audit trail is append-only, enforced in the database rather than by
-- convention. An attacker who reaches the application's database role can still
-- write misleading events, but cannot quietly erase the record of what they did.
--
-- This is strict on purpose, and it has a consequence worth stating: the
-- trigger also blocks the cascade from a deleted organization, so tenant
-- erasure and retention trimming cannot happen as a side effect of an ordinary
-- DELETE. Both are privileged procedures performed by the table owner, which is
-- not the role the application runs as. See deploy/sql/erase-organization.sql
-- and docs/security/audit.md.
CREATE OR REPLACE FUNCTION deny_audit_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$;

CREATE TRIGGER audit_events_no_update
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION deny_audit_mutation();
