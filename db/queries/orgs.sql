-- name: GetOrgID :one
SELECT id FROM orgs
WHERE public_id = $1
AND deleted_at IS NULL;

-- name: GetOrgName :one
SELECT name FROM orgs
WHERE id = $1
AND deleted_at IS NULL;

-- name: GetOrgs :many
SELECT name FROM orgs
WHERE deleted_at IS NULL;

-- name: CreateOrg :one
INSERT INTO orgs (name)
VALUES ($1)
RETURNING id, public_id, name, created_at, updated_at;

-- name: UpdateOrg :one
UPDATE orgs
SET name = $2, updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NULL
RETURNING id, public_id, name, created_at, updated_at;

-- name: SoftDeleteOrg :one
UPDATE orgs
SET deleted_at = NOW(), updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NULL
RETURNING id, public_id, name, created_at, updated_at, deleted_at;

-- name: RestoreOrg :one
UPDATE orgs
SET deleted_at = NULL, updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NOT NULL
RETURNING id, public_id, name, created_at, updated_at;