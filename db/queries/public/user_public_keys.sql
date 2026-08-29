-- name: GetUserPublicKey :one
SELECT user_id, public_key, created_at
FROM public.user_public_keys
WHERE user_id = $1;

-- name: UpsertUserPublicKey :one
INSERT INTO public.user_public_keys (user_id, public_key)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET public_key = EXCLUDED.public_key, created_at = NOW()
RETURNING user_id, public_key, created_at;

-- name: GetUserPublicKeys :many
SELECT user_id, public_key, created_at
FROM public.user_public_keys
WHERE user_id = ANY($1::uuid[]);
