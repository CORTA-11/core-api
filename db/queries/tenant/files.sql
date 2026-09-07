-- name: CreateFile :one
INSERT INTO files (team_id, name, size, content_type, object_key, iv, key_version, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, public_id, team_id, name, size, content_type, object_key, iv, key_version, uploaded_by, created_at, updated_at, deleted_at;

-- name: GetFileByID :one
SELECT id, public_id, team_id, name, size, content_type, object_key, iv, key_version, uploaded_by, created_at, updated_at, deleted_at
FROM files
WHERE team_id = $1 AND public_id = $2 AND deleted_at IS NULL;

-- name: ListFilesForTeam :many
SELECT id, public_id, team_id, name, size, content_type, object_key, iv, key_version, uploaded_by, created_at, updated_at, deleted_at
FROM files
WHERE team_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC, public_id
LIMIT sqlc.arg('limit');

-- name: SoftDeleteFile :one
UPDATE files
SET deleted_at = NOW()
WHERE team_id = $1 AND public_id = $2 AND deleted_at IS NULL
RETURNING id, public_id, team_id, name, size, content_type, object_key, iv, key_version, uploaded_by, created_at, updated_at, deleted_at;
