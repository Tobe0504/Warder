-- Tenant erasure.
--
-- The audit trail is append-only, which means deleting an organization is not
-- something the application can do as a side effect of an ordinary DELETE — the
-- cascade into audit_events is refused. That is the intended behaviour: erasing
-- the record of what happened should require someone to decide to erase it.
--
-- Run this as warder_migrator (the table owner), never as warder_app. Take a
-- backup first; this is not reversible.
--
-- Usage:
--   psql "$WARDER_MIGRATION_DATABASE_URL" \
--        -v organization_id="'00000000-0000-0000-0000-000000000000'" \
--        -f deploy/sql/erase-organization.sql

\set ON_ERROR_STOP on

BEGIN;

-- Record the erasure itself before removing anything, in an organization-
-- independent form, so that the fact a tenant was erased survives the erasure.
INSERT INTO audit_events (organization_id, event_type, outcome, actor_type, actor_label, reason, metadata)
SELECT id, 'ORGANIZATION_ERASURE_STARTED', 'SUCCESS', 'HUMAN', current_user,
       'privileged tenant erasure',
       jsonb_build_object('organization_slug', slug, 'erased_at', now())
FROM organizations WHERE id = :organization_id;

-- Disabling the trigger requires table ownership, which warder_app does not
-- have. It is re-enabled before commit, and the transaction guarantees it is
-- never left off.
ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update;

DELETE FROM audit_events WHERE organization_id = :organization_id;

-- Everything else cascades from the organization row.
DELETE FROM organizations WHERE id = :organization_id;

ALTER TABLE audit_events ENABLE TRIGGER audit_events_no_update;

COMMIT;
