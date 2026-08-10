-- name: CreateChannel :one
INSERT INTO channels (team_id, name)
VALUES ($1, $2)
RETURNING id, team_id, name, created_at;

-- name: GetDefaultChannelByTeamID :one
SELECT id, team_id, name, created_at
FROM channels
WHERE team_id = $1
  AND name = 'general'
ORDER BY created_at ASC
LIMIT 1;
