-- name: CreateSession :one
INSERT INTO sessions (user_id, refresh_token, expires_at)
VALUES ($1, $2, $3)
RETURNING id, user_id, refresh_token, is_blocked, expires_at, created_at;

-- name: GetSessionByRefreshToken :one
SELECT id, user_id, refresh_token, is_blocked, expires_at, created_at
FROM sessions
WHERE refresh_token = $1
LIMIT 1;

-- name: BlockSession :exec
UPDATE sessions
SET is_blocked = true
WHERE refresh_token = $1;

-- name: BlockSessionsByUserID :exec
UPDATE sessions
SET is_blocked = true
WHERE user_id = $1
  AND is_blocked = false;
