-- name: GetTeams :many
SELECT * FROM teams;

-- name: CreateTeam :one
INSERT INTO teams (name, slug)
VALUES ($1, $2)
RETURNING *;

-- name: GetTeamID :one
SELECT id FROM teams
WHERE slug = $1;