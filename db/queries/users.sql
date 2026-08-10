-- name: CreateUser :one
INSERT INTO users (org_id, email, password_hash, name, avatar_url, org_role)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, org_id, email, name, org_role, created_at;

-- name: GetUserByEmail :one
SELECT id, org_id, email, password_hash, name, org_role, active 
FROM users 
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, org_id, email, name, org_role, active, avatar_url
FROM users
WHERE id = $1;

-- name: ListUsersByOrg :many
SELECT id, org_id, email, name, org_role, active, avatar_url
FROM users
WHERE org_id = $1
  AND active = true
ORDER BY name ASC;