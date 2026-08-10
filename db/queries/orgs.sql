-- name: GetOrgID :one
SELECT id FROM orgs
WHERE public_id = $1;

-- name: GetOrgName :one
SELECT name FROM orgs
WHERE id = $1;

-- name: GetOrgs :many
SELECT name FROM orgs;

-- name: CreateOrg :one
INSERT INTO orgs (name)
VALUES ($1)
RETURNING id, public_id, name, created_at, updated_at;

-- name: UpdateOrg :one
UPDATE orgs
SET name = $2, updated_at = NOW()
WHERE public_id = $1
RETURNING id, public_id, name, created_at, updated_at;