-- name: CreateSession :one
INSERT INTO public.sessions (
    user_id, token_hash, user_agent, created_at, last_seen_at, absolute_expires_at
)
SELECT u.id, sqlc.arg('token_hash'), sqlc.arg('user_agent'), sqlc.arg('now'),
       sqlc.arg('now'), sqlc.arg('absolute_expires_at')
FROM public.users u
WHERE u.user_id = sqlc.arg('user_public_id') AND u.deleted_at IS NULL
RETURNING id, session_id, user_id, token_hash, user_agent, created_at,
          last_seen_at, absolute_expires_at, revoked_at;

-- name: GetSessionByTokenHash :one
SELECT s.id, s.session_id, s.user_id, s.token_hash, s.user_agent, s.created_at,
       s.last_seen_at, s.absolute_expires_at, s.revoked_at,
       u.user_id AS user_public_id, u.email, u.display_name
FROM public.sessions s
JOIN public.users u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg('token_hash')
  AND s.revoked_at IS NULL
  AND s.absolute_expires_at > sqlc.arg('now')
  AND s.last_seen_at > sqlc.arg('idle_cutoff')
  AND u.deleted_at IS NULL;

-- name: TouchSession :execrows
UPDATE public.sessions
SET last_seen_at = sqlc.arg('now')
WHERE token_hash = sqlc.arg('token_hash')
  AND revoked_at IS NULL
  AND absolute_expires_at > sqlc.arg('now')
  AND last_seen_at > sqlc.arg('idle_cutoff')
  AND last_seen_at <= sqlc.arg('touch_before');

-- name: ListUserSessions :many
SELECT s.session_id, s.user_agent, s.created_at, s.last_seen_at,
       s.absolute_expires_at, s.revoked_at
FROM public.sessions s
JOIN public.users u ON u.id = s.user_id
WHERE u.user_id = sqlc.arg('user_public_id')
  AND s.revoked_at IS NULL
  AND s.absolute_expires_at > sqlc.arg('now')
  AND s.last_seen_at > sqlc.arg('idle_cutoff')
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg('result_limit');

-- name: RevokeSessionByHash :execrows
UPDATE public.sessions
SET revoked_at = COALESCE(revoked_at, sqlc.arg('now'))
WHERE token_hash = sqlc.arg('token_hash');

-- name: RevokeUserSession :execrows
UPDATE public.sessions s
SET revoked_at = COALESCE(s.revoked_at, sqlc.arg('now'))
FROM public.users u
WHERE s.user_id = u.id
  AND u.user_id = sqlc.arg('user_public_id')
  AND s.session_id = sqlc.arg('session_public_id');

-- name: RevokeAllUserSessions :exec
UPDATE public.sessions s
SET revoked_at = COALESCE(s.revoked_at, sqlc.arg('now'))
FROM public.users u
WHERE s.user_id = u.id AND u.user_id = sqlc.arg('user_public_id');

-- name: DeleteRevokedSessions :execrows
WITH candidates AS (
    SELECT candidate_sessions.id FROM public.sessions candidate_sessions
    WHERE candidate_sessions.revoked_at IS NOT NULL
    ORDER BY candidate_sessions.revoked_at, candidate_sessions.id
    LIMIT sqlc.arg('batch_limit')
    FOR UPDATE SKIP LOCKED
)
DELETE FROM public.sessions s USING candidates c WHERE s.id = c.id;

-- name: DeleteAbsoluteExpiredSessions :execrows
WITH candidates AS (
    SELECT candidate_sessions.id FROM public.sessions candidate_sessions
    WHERE candidate_sessions.revoked_at IS NULL
      AND candidate_sessions.absolute_expires_at <= sqlc.arg('now')
    ORDER BY candidate_sessions.absolute_expires_at, candidate_sessions.id
    LIMIT sqlc.arg('batch_limit')
    FOR UPDATE SKIP LOCKED
)
DELETE FROM public.sessions s USING candidates c WHERE s.id = c.id;

-- name: DeleteIdleExpiredSessions :execrows
WITH candidates AS (
    SELECT candidate_sessions.id FROM public.sessions candidate_sessions
    WHERE candidate_sessions.revoked_at IS NULL
      AND candidate_sessions.absolute_expires_at > sqlc.arg('now')
      AND candidate_sessions.last_seen_at <= sqlc.arg('idle_cutoff')
    ORDER BY candidate_sessions.last_seen_at, candidate_sessions.id
    LIMIT sqlc.arg('batch_limit')
    FOR UPDATE SKIP LOCKED
)
DELETE FROM public.sessions s USING candidates c WHERE s.id = c.id;
