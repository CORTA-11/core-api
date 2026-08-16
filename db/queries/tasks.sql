-- name: GetTasks :many
SELECT * FROM tasks
WHERE team_id = $1
ORDER BY created_at ASC;

-- name: CreateTask :one
INSERT INTO tasks (team_id, description)
VALUES ($1, $2)
RETURNING *;