-- name: UpsertTeamSharedKey :one
INSERT INTO team_shared_keys (team_id, user_id, encrypted_key, key_version)
VALUES ($1, $2, $3, $4)
ON CONFLICT (team_id, user_id, key_version) DO UPDATE
SET encrypted_key = EXCLUDED.encrypted_key, created_at = NOW()
RETURNING team_id, user_id, encrypted_key, key_version, created_at;

-- name: GetTeamSharedKeyForUser :one
SELECT team_id, user_id, encrypted_key, key_version, created_at
FROM team_shared_keys
WHERE team_id = $1 AND user_id = $2 AND key_version = $3;

-- name: ListTeamSharedKeysForUser :many
SELECT team_id, user_id, encrypted_key, key_version, created_at
FROM team_shared_keys
WHERE team_id = $1 AND user_id = $2
ORDER BY key_version DESC;

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
ORDER BY created_at DESC;

-- name: SoftDeleteFile :one
UPDATE files
SET deleted_at = NOW()
WHERE team_id = $1 AND public_id = $2 AND deleted_at IS NULL
RETURNING id, public_id, team_id, name, size, content_type, object_key, iv, key_version, uploaded_by, created_at, updated_at, deleted_at;
