-- name: ListChatMessages :many
SELECT
    m.id,
    m.channel_id,
    m.sender_id,
    m.reply_to_id,
    CASE WHEN m.deleted_at IS NULL THEN m.message ELSE '' END AS message,
    m.created_at,
    m.deleted_at,
    u.name AS sender_name,
    u.avatar_url AS sender_avatar_url
FROM chat_messages m
INNER JOIN users u ON u.id = m.sender_id
WHERE m.channel_id = $1
  AND (
    sqlc.narg('before')::uuid IS NULL
    OR (m.created_at, m.id) < (
        SELECT cm.created_at, cm.id
        FROM chat_messages cm
        WHERE cm.id = sqlc.narg('before')
    )
  )
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg('message_limit');

-- name: GetChatMessageByID :one
SELECT
    id,
    channel_id,
    sender_id,
    reply_to_id,
    message,
    created_at,
    deleted_at
FROM chat_messages
WHERE id = $1;

-- name: CreateChatMessage :one
INSERT INTO chat_messages (channel_id, sender_id, reply_to_id, message)
VALUES ($1, $2, $3, $4)
RETURNING id, channel_id, sender_id, reply_to_id, message, created_at, deleted_at;

-- name: SoftDeleteChatMessage :one
UPDATE chat_messages
SET deleted_at = now()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING id, channel_id, sender_id, reply_to_id, message, created_at, deleted_at;
