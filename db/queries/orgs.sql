-- name: GetOrgID :one
SELECT id FROM orgs
WHERE public_id = $1
AND deleted_at IS NULL;

-- name: GetOrgName :one
SELECT * FROM orgs
WHERE id = $1
AND deleted_at IS NULL;

-- name: GetOrgs :many
SELECT * FROM orgs
WHERE deleted_at is NULL
ORDER BY name ASC;

-- name: CreateOrg :one
INSERT INTO orgs (name, public_id, schema_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateOrg :one
UPDATE orgs
SET name = $2, updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteOrg :one
UPDATE orgs
SET deleted_at = NOW(), updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NULL
RETURNING *;

-- name: RestoreOrg :one
UPDATE orgs
SET deleted_at = NULL, updated_at = NOW()
WHERE public_id = $1
AND deleted_at IS NOT NULL
RETURNING *;

-- name: GetSchemaFromID :one
SELECT schema_name FROM orgs
WHERE id = $1;