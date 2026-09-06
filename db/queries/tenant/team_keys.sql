-- name: CreateTeamKeyVersion :one
SELECT id, team_id, version, status, algorithm, wraps, created_by, created_at
FROM synodus_commit_team_key($1, $2, $3, $4);

-- name: ListTeamKeysForTeam :many
SELECT id, team_id, version, status, algorithm, wraps, created_by, created_at
FROM team_keys
WHERE team_id = $1
ORDER BY version DESC
LIMIT sqlc.arg('limit');