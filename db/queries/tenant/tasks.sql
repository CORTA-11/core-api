-- name: GetTasks :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at
FROM tasks
JOIN teams ON teams.id = tasks.team_id
WHERE teams.id = sqlc.arg('team_id')
ORDER BY tasks.created_at ASC, tasks.id ASC
LIMIT sqlc.arg('limit');

-- name: CreateTask :one
INSERT INTO tasks (team_id, description, status)
VALUES ($1, $2, $3)
RETURNING id, team_id, description, status, created_at, updated_at;

-- name: UpdateTask :one
UPDATE tasks
SET description = $3,
    status = $4,
    updated_at = NOW()
WHERE id = $1 AND team_id = $2
RETURNING id, team_id, description, status, created_at, updated_at;

-- name: DeleteTask :one
DELETE FROM tasks
WHERE id = $1 AND team_id = $2
RETURNING id, team_id, description, status, created_at, updated_at;
