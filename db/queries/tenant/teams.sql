-- name: GetTeams :many
SELECT id, name, slug, created_at, updated_at, public_id, is_quarantine
FROM teams
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: CreateTeam :one
INSERT INTO teams (name, slug)
VALUES ($1, $2)
RETURNING id, name, slug, created_at, updated_at, public_id, is_quarantine;

-- name: GetTeamID :one
SELECT id
FROM teams
WHERE slug = $1;
