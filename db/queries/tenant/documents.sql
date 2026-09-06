-- name: CreateDocument :one
INSERT INTO documents (team_id, title, last_updated_by)
VALUES (sqlc.arg('team_id'), sqlc.arg('title'), sqlc.arg('last_updated_by'))
RETURNING id, public_id, team_id, canonical_state, title, body_html, last_updated_by, created_at, updated_at;

-- name: ListDocumentsForTeam :many
SELECT id, public_id, team_id, title, body_html, last_updated_by, created_at, updated_at
FROM documents
WHERE team_id = sqlc.arg('team_id')
ORDER BY updated_at DESC, public_id DESC
LIMIT sqlc.arg('limit');
