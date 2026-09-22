-- name: UpsertDeviceToken :one
INSERT INTO public.user_device_tokens (user_id, token, platform)
VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE
SET user_id = EXCLUDED.user_id,
    platform = EXCLUDED.platform,
    updated_at = NOW()
RETURNING id, user_id, token, platform, created_at, updated_at;

-- name: GetDeviceTokensForUsers :many
SELECT user_id, token, platform
FROM public.user_device_tokens
WHERE user_id = ANY(sqlc.arg('user_ids')::uuid[]);

-- name: DeleteDeviceToken :exec
DELETE FROM public.user_device_tokens
WHERE token = $1;
