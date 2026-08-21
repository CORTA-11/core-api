-- Refuse to discard the registry's reconciliation history after tenant schemas
-- depend on it; rollback must preserve state for forward repair.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.orgs
        WHERE tenant_version <> 0 OR tenant_checksum <> '' OR lifecycle_state = 'active'
    ) THEN
        RAISE EXCEPTION 'cannot remove organization lifecycle while tenant reconciliation state exists';
    END IF;
END $$;

DROP INDEX IF EXISTS public.orgs_reconciliation_due_idx;
ALTER TABLE public.orgs
    DROP CONSTRAINT orgs_deletion_state_check,
    DROP CONSTRAINT orgs_failed_inspectable_check,
    DROP CONSTRAINT orgs_active_reconciled_check,
    DROP CONSTRAINT orgs_canonical_schema_check,
    DROP CONSTRAINT orgs_reconcile_attempts_check,
    DROP CONSTRAINT orgs_lifecycle_state_check,
    DROP COLUMN provisioned_at,
    DROP COLUMN last_attempt_at,
    DROP COLUMN last_error_detail,
    DROP COLUMN last_error_code,
    DROP COLUMN next_attempt_at,
    DROP COLUMN reconcile_attempts,
    DROP COLUMN tenant_checksum,
    DROP COLUMN tenant_version,
    DROP COLUMN lifecycle_state;
