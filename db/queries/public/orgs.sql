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

-- name: ListOrganizationsForUserAfter :many
SELECT organization.id, organization.public_id, organization.name, organization.schema_name,
       organization.created_at, organization.updated_at, organization.deleted_at,
       organization.lifecycle_state, organization.tenant_version, organization.tenant_checksum,
       organization.reconcile_attempts, organization.next_attempt_at,
       organization.last_error_code, organization.last_error_detail,
       organization.last_attempt_at, organization.provisioned_at
FROM public.orgs AS organization
JOIN public.org_user AS membership ON membership.org_id = organization.id
JOIN public.users AS app_user ON app_user.id = membership.user_id
WHERE app_user.user_id = sqlc.arg('user_public_id')
  AND app_user.deleted_at IS NULL
  AND (organization.created_at, organization.public_id) >
      (sqlc.arg('after_created_at'), sqlc.arg('after_public_id')::uuid)
ORDER BY organization.created_at ASC, organization.public_id ASC
LIMIT sqlc.arg('limit');

-- name: ListOrganizationsForUserBefore :many
SELECT organization.id, organization.public_id, organization.name, organization.schema_name,
       organization.created_at, organization.updated_at, organization.deleted_at,
       organization.lifecycle_state, organization.tenant_version, organization.tenant_checksum,
       organization.reconcile_attempts, organization.next_attempt_at,
       organization.last_error_code, organization.last_error_detail,
       organization.last_attempt_at, organization.provisioned_at
FROM public.orgs AS organization
JOIN public.org_user AS membership ON membership.org_id = organization.id
JOIN public.users AS app_user ON app_user.id = membership.user_id
WHERE app_user.user_id = sqlc.arg('user_public_id')
  AND app_user.deleted_at IS NULL
  AND (organization.created_at, organization.public_id) <
      (sqlc.arg('before_created_at'), sqlc.arg('before_public_id')::uuid)
ORDER BY organization.created_at DESC, organization.public_id DESC
LIMIT sqlc.arg('limit');

-- name: GetOrganizationByPublicIDIncludingDeleted :one
SELECT id, public_id, name, schema_name, created_at, updated_at, deleted_at,
       lifecycle_state, tenant_version, tenant_checksum,
       reconcile_attempts, next_attempt_at, last_error_code, last_error_detail,
       last_attempt_at, provisioned_at
FROM public.orgs
WHERE public_id = sqlc.arg('public_id');

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

-- name: UpdateOrganizationIncludingDeleting :one
UPDATE public.orgs
SET name = sqlc.arg('name'), updated_at = NOW()
WHERE public_id = sqlc.arg('public_id')
  AND lifecycle_state <> 'deleted'
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
