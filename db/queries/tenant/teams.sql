-- name: GetTeams :many
SELECT id, name, slug, created_at, updated_at
FROM teams
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: CreateTeam :one
INSERT INTO teams (name, slug)
VALUES ($1, $2)
RETURNING id, name, slug, created_at, updated_at;

-- name: GetTeamID :one
SELECT id
FROM teams
WHERE slug = $1;
