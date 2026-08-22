-- name: ResolveOrganizationContext :one
SELECT organization.id AS organization_id,
       organization.public_id AS organization_public_id,
       organization.schema_name,
       organization.lifecycle_state,
       organization.tenant_version,
       organization.tenant_checksum,
       app_user.id AS user_id,
       app_user.user_id AS user_public_id,
       (namespace.oid IS NOT NULL)::boolean AS schema_exists
FROM public.users AS app_user
JOIN public.org_user AS membership ON membership.user_id = app_user.id
JOIN public.orgs AS organization ON organization.id = membership.org_id
LEFT JOIN pg_catalog.pg_namespace AS namespace ON namespace.nspname = organization.schema_name
WHERE app_user.user_id = sqlc.arg('user_public_id')
  AND app_user.deleted_at IS NULL
  AND organization.public_id = sqlc.arg('organization_public_id')
  AND organization.deleted_at IS NULL;
