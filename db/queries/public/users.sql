-- name: GetAllUsers :many
SELECT id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical
FROM public.users
WHERE deleted_at IS NULL
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: GetUserByID :one
SELECT id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical
FROM public.users
WHERE user_id = $1
AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO public.users (email, password_hash, display_name)
VALUES ($1, $2, $3)
RETURNING id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical;

-- name: GetUserByCanonicalEmail :one
SELECT id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical
FROM public.users
WHERE email_canonical = $1
AND deleted_at IS NULL;

-- name: UpdateUser :one
UPDATE public.users
SET email = $2, password_hash = $3, display_name = $4, updated_at = NOW()
WHERE user_id = $1
AND deleted_at IS NULL
RETURNING id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical;

-- name: SoftDeleteUser :one
UPDATE public.users
SET deleted_at = NOW()
WHERE user_id = $1
AND deleted_at IS NULL
RETURNING id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical;
