-- name: ListChatMessages :many
SELECT public_id, team_id, sender_user_public_id, reply_to_public_id,
       mentions, message, created_at, deleted_at
FROM chat_messages
WHERE team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
  AND (NOT sqlc.arg('before_set')::boolean OR created_at < sqlc.arg('before_created_at')::timestamptz)
ORDER BY created_at DESC, public_id DESC
LIMIT sqlc.arg('limit');

-- name: CreateChatMessage :one
INSERT INTO chat_messages (team_id, sender_user_public_id, reply_to_public_id, mentions, message)
SELECT NULLIF(current_setting('app.team_id', true), '')::BIGINT,
       synodus_app_user_public_id(),
       sqlc.narg('reply_to_public_id'),
       sqlc.arg('mentions')::uuid[],
       sqlc.arg('message')
WHERE sqlc.narg('reply_to_public_id')::uuid IS NULL
   OR EXISTS (
       SELECT 1
       FROM chat_messages AS reply
       WHERE reply.team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
         AND reply.public_id = sqlc.narg('reply_to_public_id')
   )
RETURNING public_id, team_id, sender_user_public_id, reply_to_public_id,
          mentions, message, created_at, deleted_at;

-- name: SoftDeleteChatMessage :one
UPDATE chat_messages AS message
SET deleted_at = now()
WHERE message.team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
  AND message.public_id = sqlc.arg('public_id')
  AND message.deleted_at IS NULL
RETURNING message.public_id, message.team_id, message.sender_user_public_id,
          message.reply_to_public_id, message.mentions, message.message,
          message.created_at, message.deleted_at;
