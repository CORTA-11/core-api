-- name: GetOrgID :one
SELECT id
FROM public.orgs
WHERE public_id = $1
AND deleted_at IS NULL;

-- name: GetOrgName :one
SELECT id, public_id, name, schema_name, created_at, updated_at, deleted_at,
       lifecycle_state, tenant_version, tenant_checksum,
       reconcile_attempts, next_attempt_at, last_error_code, last_error_detail,
       last_attempt_at, provisioned_at
FROM public.orgs
WHERE id = $1
AND deleted_at IS NULL;

-- name: GetOrgs :many
SELECT id, public_id, name, schema_name, created_at, updated_at, deleted_at,
       lifecycle_state, tenant_version, tenant_checksum,
       reconcile_attempts, next_attempt_at, last_error_code, last_error_detail,
       last_attempt_at, provisioned_at
FROM public.orgs
WHERE deleted_at IS NULL
ORDER BY name ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: CreateOrg :one
INSERT INTO public.orgs (name, public_id, schema_name)
VALUES ($1, $2, $3)
RETURNING id, public_id, name, schema_name, created_at, updated_at, deleted_at,
          lifecycle_state, tenant_version, tenant_checksum,
          reconcile_attempts, next_attempt_at, last_error_code, last_error_detail,
          last_attempt_at, provisioned_at;

-- name: UpdateOrg :one
UPDATE public.orgs
SET name = $2, updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NULL
RETURNING id, public_id, name, schema_name, created_at, updated_at, deleted_at,
          lifecycle_state, tenant_version, tenant_checksum,
          reconcile_attempts, next_attempt_at, last_error_code, last_error_detail,
          last_attempt_at, provisioned_at;

-- name: SoftDeleteOrg :one
UPDATE public.orgs
SET deleted_at = NOW(), lifecycle_state = 'deleting', next_attempt_at = NOW(), updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NULL
RETURNING id, public_id, name, schema_name, created_at, updated_at, deleted_at,
          lifecycle_state, tenant_version, tenant_checksum,
          reconcile_attempts, next_attempt_at, last_error_code, last_error_detail,
          last_attempt_at, provisioned_at;

-- name: RestoreOrg :one
UPDATE public.orgs
SET deleted_at = NULL, lifecycle_state = 'provisioning', reconcile_attempts = 0,
    next_attempt_at = NOW(), last_error_code = NULL, last_error_detail = NULL,
    updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NOT NULL
RETURNING id, public_id, name, schema_name, created_at, updated_at, deleted_at,
          lifecycle_state, tenant_version, tenant_checksum,
          reconcile_attempts, next_attempt_at, last_error_code, last_error_detail,
          last_attempt_at, provisioned_at;
