-- name: GetTasks :many
SELECT tasks.*
FROM tasks
JOIN teams ON teams.id = tasks.team_id
WHERE teams.id = $1
ORDER BY tasks.created_at ASC;

-- name: CreateTask :one
INSERT INTO tasks (team_id, description)
VALUES ($1, $2)
RETURNING *;