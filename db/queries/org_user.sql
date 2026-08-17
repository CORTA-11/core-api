-- name: GetNumberOfUsersInOrg :one
SELECT COUNT(*) FROM org_user
WHERE org_id = $1;

-- name: GetUsersInOrg :many
SELECT u.* FROM users u
JOIN org_user ou ON u.id = ou.user_id
WHERE ou.org_id = $1
AND u.deleted_at IS NULL;

-- name: AddUserToOrg :one
INSERT INTO org_user (org_id, user_id)
VALUES ($1, $2)
RETURNING *;

-- name: RemoveUserFromOrg :one
DELETE FROM org_user
WHERE org_id = $1
AND user_id = $2
RETURNING *;

-- name: GetOrgsForUser :many
SELECT o.* FROM orgs o
JOIN org_user ou ON o.id = ou.org_id
WHERE ou.user_id = $1
AND o.deleted_at IS NULL;   

