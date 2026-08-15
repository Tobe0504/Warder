-- Least-privilege database roles.
--
-- Run this once, as a superuser, after migrations have been applied. It splits
-- three concerns that are usually collapsed into a single database user:
--
--   warder_migrator   owns the schema, applies migrations, and is the only role
--                     that can drop the audit trigger. Used by the migrate
--                     command and by nothing else.
--
--   warder_app        the role the API runs as. Reads and writes rows. Cannot
--                     alter the schema, cannot disable the audit trigger, and
--                     therefore cannot rewrite history.
--
--   warder_readonly   metadata only. Reporting, support tooling, and analytics
--                     replicas use this. It has no privilege whatsoever on the
--                     secret_material schema, so a leaked reporting credential
--                     exposes which credentials exist but no ciphertext.
--
-- Replace the passwords below before running. They are placeholders.

-- ---------------------------------------------------------------------------
-- Roles
-- ---------------------------------------------------------------------------

CREATE ROLE warder_migrator LOGIN PASSWORD 'replace-me-migrator';
CREATE ROLE warder_app      LOGIN PASSWORD 'replace-me-app';
CREATE ROLE warder_readonly LOGIN PASSWORD 'replace-me-readonly';

-- The migrator owns everything, so only it can ALTER or DROP.
ALTER SCHEMA public          OWNER TO warder_migrator;
ALTER SCHEMA secret_material OWNER TO warder_migrator;

DO $$
DECLARE t record;
BEGIN
    FOR t IN
        SELECT schemaname, tablename FROM pg_tables
        WHERE schemaname IN ('public', 'secret_material')
    LOOP
        EXECUTE format('ALTER TABLE %I.%I OWNER TO warder_migrator', t.schemaname, t.tablename);
    END LOOP;
END;
$$;

-- ---------------------------------------------------------------------------
-- Application role
-- ---------------------------------------------------------------------------

GRANT USAGE ON SCHEMA public, secret_material TO warder_app;

GRANT SELECT, INSERT, UPDATE, DELETE
    ON ALL TABLES IN SCHEMA public TO warder_app;

-- Secret material is written once and read; it is never updated in place.
-- Rotation writes a new version rather than overwriting an old one, so the
-- application has no legitimate reason to hold UPDATE here. Withholding it
-- means a compromised application process cannot silently swap the ciphertext
-- under an existing version that other systems believe they have audited.
GRANT SELECT, INSERT, DELETE
    ON ALL TABLES IN SCHEMA secret_material TO warder_app;

-- The application must be able to write audit events but never revise them.
-- The trigger already refuses UPDATE and DELETE; this removes the privilege as
-- well, so the refusal does not depend on the trigger surviving.
REVOKE UPDATE, DELETE ON audit_events FROM warder_app;

-- The application does not manage its own migration bookkeeping.
REVOKE ALL ON schema_migrations FROM warder_app;
GRANT SELECT ON schema_migrations TO warder_app;

-- ---------------------------------------------------------------------------
-- Read-only metadata role
-- ---------------------------------------------------------------------------

GRANT USAGE ON SCHEMA public TO warder_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO warder_readonly;

-- Explicitly not granted: any privilege on secret_material, and the password
-- hash column. Column-level revocation keeps a reporting query from pulling
-- credential verifiers into a warehouse.
REVOKE SELECT ON users FROM warder_readonly;
GRANT SELECT (id, email, name, created_at, updated_at, disabled_at)
    ON users TO warder_readonly;

REVOKE SELECT ON user_sessions, machine_tokens, runtime_sessions FROM warder_readonly;

-- Invitations are useful to report on — who was invited, by whom, whether it
-- was accepted — but the row also holds the verifier for a live invitation.
-- Everything except that column.
REVOKE SELECT ON membership_invitations FROM warder_readonly;
GRANT SELECT (id, organization_id, email, name, role, membership_expires_at,
              public_id, created_at, created_by, expires_at, accepted_at,
              accepted_user_id, revoked_at)
    ON membership_invitations TO warder_readonly;

-- ---------------------------------------------------------------------------
-- Defaults for tables added by future migrations
-- ---------------------------------------------------------------------------

ALTER DEFAULT PRIVILEGES FOR ROLE warder_migrator IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO warder_app;
ALTER DEFAULT PRIVILEGES FOR ROLE warder_migrator IN SCHEMA public
    GRANT SELECT ON TABLES TO warder_readonly;
ALTER DEFAULT PRIVILEGES FOR ROLE warder_migrator IN SCHEMA secret_material
    GRANT SELECT, INSERT, DELETE ON TABLES TO warder_app;
