-- name: GetTasks :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at, tasks.public_id
FROM tasks
ORDER BY tasks.created_at ASC, tasks.id ASC
LIMIT sqlc.arg('limit');

-- name: CreateTask :one
INSERT INTO tasks (team_id, description, status)
VALUES (NULLIF(current_setting('app.team_id', true), '')::BIGINT, $1, $2)
RETURNING id, team_id, description, status, created_at, updated_at, public_id;

-- name: UpdateTask :one
UPDATE tasks
SET description = $2,
    status = $3,
    updated_at = NOW()
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id;

-- name: DeleteTask :one
DELETE FROM tasks
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id;

-- name: IsolationProbeTasks :many
-- Deliberately omits a team predicate: FORCE RLS is the isolation mechanism
-- under proof. Keep this query bounded and out of production service paths.
SELECT id, team_id, description, status, created_at, updated_at, public_id
FROM tasks
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');
