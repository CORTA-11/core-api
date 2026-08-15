-- name: GetTeams :many
SELECT * FROM teams;

-- name: CreateTeam :one
INSERT INTO teams (name)
VALUES ($1)
RETURNING *;