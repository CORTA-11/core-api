-- name: GetAllUsers :many
SELECT * FROM users
WHERE deleted_at IS NULL;


-- name: GetUserByID :one
SELECT * FROM users
WHERE user_id = $1 
AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1
AND deleted_at IS NULL;

-- name: UpdateUser :one
UPDATE users
SET email = $2, password_hash = $3, display_name = $4, updated_at = NOW()
WHERE user_id = $1
AND deleted_at IS NULL
RETURNING *;


-- name: SoftDeleteUser :one
UPDATE users
SET deleted_at = NOW()
WHERE user_id = $1
AND deleted_at IS NULL
RETURNING *;


