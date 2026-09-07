-- name: GetUserPublicKey :one
SELECT user_id, public_key, encrypted_private_key, kek_salt, kek_iterations, kek_algorithm, created_at, updated_at
FROM public.user_public_keys
WHERE user_id = $1;

-- name: UpsertUserKeys :one
INSERT INTO public.user_public_keys (user_id, public_key, encrypted_private_key, kek_salt, kek_iterations, kek_algorithm)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id) DO UPDATE
SET public_key = EXCLUDED.public_key,
    encrypted_private_key = COALESCE(EXCLUDED.encrypted_private_key, user_public_keys.encrypted_private_key),
    kek_salt = COALESCE(EXCLUDED.kek_salt, user_public_keys.kek_salt),
    kek_iterations = COALESCE(EXCLUDED.kek_iterations, user_public_keys.kek_iterations),
    kek_algorithm = COALESCE(EXCLUDED.kek_algorithm, user_public_keys.kek_algorithm),
    updated_at = NOW()
RETURNING user_id, public_key, encrypted_private_key, kek_salt, kek_iterations, kek_algorithm, created_at, updated_at;

-- name: GetUserPublicKeys :many
SELECT user_id, public_key, created_at
FROM public.user_public_keys
WHERE user_id = ANY(sqlc.arg('user_ids')::uuid[])
ORDER BY user_id
LIMIT sqlc.arg('limit');