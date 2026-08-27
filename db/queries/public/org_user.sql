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
RETURNING org_id, user_id, role, created_at, updated_at;

-- name: RemoveUserFromOrg :one
DELETE FROM public.org_user
WHERE org_id = $1
AND user_id = $2
RETURNING org_id, user_id, role, created_at, updated_at;

-- name: GetOrganizationMembership :one
SELECT membership.org_id, membership.user_id, membership.role,
       membership.created_at, membership.updated_at
FROM public.org_user AS membership
JOIN public.orgs AS organization ON organization.id = membership.org_id
JOIN public.users AS app_user ON app_user.id = membership.user_id
WHERE organization.public_id = sqlc.arg('organization_public_id')
  AND organization.deleted_at IS NULL
  AND app_user.user_id = sqlc.arg('user_public_id')
  AND app_user.deleted_at IS NULL;

-- name: AssignOrganizationOwner :one
WITH locked_organization AS (
    SELECT id, public_id
    FROM public.orgs
    WHERE public_id = sqlc.arg('organization_public_id')
      AND deleted_at IS NULL
      AND lifecycle_state = 'active'
    FOR UPDATE
), active_user AS (
    SELECT app_user.id, app_user.user_id
    FROM public.users AS app_user
    WHERE app_user.user_id = sqlc.arg('user_public_id')
      AND app_user.deleted_at IS NULL
), assigned AS (
    INSERT INTO public.org_user AS membership (org_id, user_id, role)
    SELECT locked_organization.id, active_user.id, 'owner'
    FROM locked_organization CROSS JOIN active_user
    ON CONFLICT (org_id, user_id) DO UPDATE
    SET role = 'owner', updated_at = now()
    RETURNING membership.org_id, membership.user_id, membership.role
)
SELECT locked_organization.public_id AS organization_public_id,
       active_user.user_id AS user_public_id,
       assigned.role
FROM assigned
JOIN locked_organization ON locked_organization.id = assigned.org_id
JOIN active_user ON active_user.id = assigned.user_id;

-- name: LockOrganizationMemberships :many
SELECT membership.org_id, membership.user_id, membership.role,
       membership.created_at, membership.updated_at
FROM public.orgs AS organization
JOIN public.org_user AS membership ON membership.org_id = organization.id
WHERE organization.public_id = sqlc.arg('organization_public_id')
ORDER BY membership.user_id
LIMIT sqlc.arg('limit')
FOR UPDATE OF organization, membership;

-- name: CountOrganizationOwners :one
SELECT count(*)
FROM public.org_user
WHERE org_id = sqlc.arg('org_id') AND role = 'owner';

-- name: SetOrganizationMembershipRole :one
UPDATE public.org_user
SET role = sqlc.arg('role'), updated_at = now()
WHERE org_id = sqlc.arg('org_id') AND user_id = sqlc.arg('user_id')
RETURNING org_id, user_id, role, created_at, updated_at;

-- name: AddOrganizationOwnerMembership :one
INSERT INTO public.org_user (org_id, user_id, role)
VALUES (sqlc.arg('org_id'), sqlc.arg('user_id'), 'owner')
RETURNING org_id, user_id, role, created_at, updated_at;

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
