-- name: GetNumberOfUsersInOrg :one
SELECT COUNT(*)
FROM public.org_user
WHERE org_id = $1;

-- name: GetUsersInOrg :many
SELECT u.id, u.user_id, u.email, u.password_hash, u.display_name, u.created_at, u.updated_at, u.deleted_at, u.email_canonical, u.password_normalization
FROM public.users AS u
JOIN public.org_user AS ou ON u.id = ou.user_id
WHERE ou.org_id = sqlc.arg('org_id')
AND u.deleted_at IS NULL
ORDER BY u.created_at ASC, u.id ASC
LIMIT sqlc.arg('limit');

-- name: AddUserToOrg :one
INSERT INTO public.org_user (org_id, user_id)
VALUES ($1, $2)
RETURNING org_id, user_id;

-- name: RemoveUserFromOrg :one
DELETE FROM public.org_user
WHERE org_id = $1
AND user_id = $2
RETURNING org_id, user_id;

-- name: GetOrgsForUser :many
SELECT o.id, o.public_id, o.name, o.schema_name, o.created_at, o.updated_at, o.deleted_at,
       o.lifecycle_state, o.tenant_version,
       o.tenant_checksum, o.reconcile_attempts, o.next_attempt_at, o.last_error_code,
       o.last_error_detail, o.last_attempt_at, o.provisioned_at
FROM public.orgs AS o
JOIN public.org_user AS ou ON o.id = ou.org_id
WHERE ou.user_id = sqlc.arg('user_id')
AND o.deleted_at IS NULL
ORDER BY o.name ASC, o.id ASC
LIMIT sqlc.arg('limit');
