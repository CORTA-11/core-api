-- Canonicalize registry-only names, but refuse to detach an already-created
-- tenant schema from its registry row. Such schemas require operator repair.
DO $$
DECLARE
    organization RECORD;
    canonical_name TEXT;
BEGIN
    FOR organization IN SELECT public_id, schema_name FROM public.orgs LOOP
        canonical_name := 'org_' || replace(organization.public_id::text, '-', '');
        IF organization.schema_name <> canonical_name THEN
            IF to_regnamespace(organization.schema_name) IS NOT NULL THEN
                RAISE EXCEPTION 'organization % has an existing noncanonical tenant schema', organization.public_id
                    USING ERRCODE = 'check_violation';
            END IF;
            UPDATE public.orgs
            SET schema_name = canonical_name
            WHERE public_id = organization.public_id;
        END IF;
    END LOOP;
END $$;

ALTER TABLE public.orgs
    ADD COLUMN lifecycle_state TEXT NOT NULL DEFAULT 'provisioning',
    ADD COLUMN tenant_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN tenant_checksum CHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN reconcile_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN last_error_code VARCHAR(64),
    ADD COLUMN last_error_detail VARCHAR(512),
    ADD COLUMN last_attempt_at TIMESTAMPTZ,
    ADD COLUMN provisioned_at TIMESTAMPTZ;

UPDATE public.orgs
SET lifecycle_state = CASE WHEN deleted_at IS NULL THEN 'provisioning' ELSE 'deleting' END;

-- These constraints make availability, failure diagnostics, canonical schema
-- ownership, and deletion mutually consistent even outside application code.
ALTER TABLE public.orgs
    ADD CONSTRAINT orgs_lifecycle_state_check
        CHECK (lifecycle_state IN ('provisioning', 'active', 'failed', 'deleting')),
    ADD CONSTRAINT orgs_reconcile_attempts_check
        CHECK (reconcile_attempts >= 0),
    ADD CONSTRAINT orgs_canonical_schema_check
        CHECK (schema_name = 'org_' || replace(public_id::text, '-', '')),
    ADD CONSTRAINT orgs_active_reconciled_check
        CHECK (lifecycle_state <> 'active' OR
            (tenant_version > 0 AND tenant_checksum <> '' AND provisioned_at IS NOT NULL
             AND last_error_code IS NULL AND last_error_detail IS NULL)),
    ADD CONSTRAINT orgs_failed_inspectable_check
        CHECK (lifecycle_state <> 'failed' OR
            (last_error_code IS NOT NULL AND last_error_detail IS NOT NULL AND last_attempt_at IS NOT NULL)),
    ADD CONSTRAINT orgs_deletion_state_check
        CHECK ((deleted_at IS NULL) = (lifecycle_state <> 'deleting'));

-- The provisioner scans only retryable rows ordered by their bounded claim time.
CREATE INDEX orgs_reconciliation_due_idx
    ON public.orgs (next_attempt_at, id)
    WHERE deleted_at IS NULL AND lifecycle_state = 'provisioning';
