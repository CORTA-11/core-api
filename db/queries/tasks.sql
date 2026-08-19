-- name: GetTasks :many
SELECT tasks.*
FROM tasks
JOIN teams ON teams.id = tasks.team_id
WHERE teams.id = $1
ORDER BY tasks.created_at ASC;

-- name: CreateTask :one
INSERT INTO tasks (team_id, description, status)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateTask :one
UPDATE tasks
SET description = $3,
    status = $4,
    updated_at = NOW()
WHERE id = $1 AND team_id = $2
RETURNING *;

-- name: DeleteTask :one
DELETE FROM tasks
WHERE id = $1 AND team_id = $2
RETURNING *;