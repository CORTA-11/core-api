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

-- name: GetDocumentForTeam :one
SELECT id, public_id, team_id, canonical_state, title, body_html, last_updated_by, created_at, updated_at
FROM documents
WHERE team_id = sqlc.arg('team_id') AND public_id = sqlc.arg('public_id');

-- name: UpdateDocument :one
UPDATE documents
SET title = COALESCE(sqlc.narg('title'), title),
    body_html = COALESCE(sqlc.narg('body_html'), body_html),
    last_updated_by = sqlc.arg('last_updated_by'),
    updated_at = NOW()
WHERE public_id = sqlc.arg('public_id')
RETURNING id, public_id, team_id, canonical_state, title, body_html, last_updated_by, created_at, updated_at;

-- name: DeleteDocument :execrows
DELETE FROM documents
WHERE public_id = sqlc.arg('public_id');
