-- name: GetAllUsers :many
SELECT id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical, password_normalization
FROM public.users
WHERE deleted_at IS NULL
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: GetUserByID :one
SELECT id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical, password_normalization
FROM public.users
WHERE user_id = $1
AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO public.users (email, password_hash, display_name, password_normalization)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical, password_normalization;

-- name: GetUserByCanonicalEmail :one
SELECT id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical, password_normalization
FROM public.users
WHERE email_canonical = $1
AND deleted_at IS NULL;

-- name: UpdateUser :one
UPDATE public.users
SET email = $2, password_hash = $3, display_name = $4,
    password_normalization = $5, updated_at = NOW()
WHERE user_id = $1
AND deleted_at IS NULL
RETURNING id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical, password_normalization;

-- name: SoftDeleteUser :one
UPDATE public.users
SET deleted_at = NOW()
WHERE user_id = $1
AND deleted_at IS NULL
RETURNING id, user_id, email, password_hash, display_name, created_at, updated_at, deleted_at, email_canonical, password_normalization;

-- name: GetCredentialByCanonicalEmail :one
SELECT user_id, password_hash, password_normalization, deleted_at
FROM public.users
WHERE email_canonical = $1;

-- name: GetCurrentCredentialByUserID :one
SELECT user_id, password_hash, password_normalization, deleted_at
FROM public.users
WHERE user_id = $1;

-- name: CompareAndSwapCredential :execrows
UPDATE public.users
SET password_hash = sqlc.arg('new_hash'),
    password_normalization = sqlc.arg('new_normalization'),
    updated_at = NOW()
WHERE user_id = sqlc.arg('user_id')
  AND password_hash = sqlc.arg('expected_hash')
  AND password_normalization = sqlc.arg('expected_normalization')
  AND deleted_at IS NULL;
