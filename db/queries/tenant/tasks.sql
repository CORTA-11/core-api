-- name: GetTasks :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at, tasks.public_id, tasks.assignee_public_id
FROM tasks
ORDER BY tasks.created_at ASC, tasks.id ASC
LIMIT sqlc.arg('limit');

-- name: GetTasksAfter :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at, tasks.public_id, tasks.assignee_public_id
FROM tasks
WHERE (tasks.created_at, tasks.public_id) > (sqlc.arg('after_created_at'), sqlc.arg('after_public_id')::uuid)
ORDER BY tasks.created_at ASC, tasks.public_id ASC
LIMIT sqlc.arg('limit');

-- name: GetTasksBefore :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at, tasks.public_id, tasks.assignee_public_id
FROM tasks
WHERE (tasks.created_at, tasks.public_id) < (sqlc.arg('before_created_at'), sqlc.arg('before_public_id')::uuid)
ORDER BY tasks.created_at DESC, tasks.public_id DESC
LIMIT sqlc.arg('limit');

-- name: CreateTask :one
INSERT INTO tasks (team_id, description, status, assignee_public_id)
VALUES (NULLIF(current_setting('app.team_id', true), '')::BIGINT, $1, $2, $3)
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id;

-- name: UpdateTask :one
UPDATE tasks
SET description = $2,
    status = $3,
    assignee_public_id = COALESCE($4, assignee_public_id),
    updated_at = NOW()
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id;

-- name: UnassignTask :one
UPDATE tasks
SET assignee_public_id = NULL,
    updated_at = NOW()
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id;

-- name: AssigneeIsMember :one
SELECT EXISTS (
    SELECT 1 FROM team_members
    WHERE team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
      AND user_public_id = $1
);

-- name: DeleteTask :one
DELETE FROM tasks
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id;

-- name: IsolationProbeTasks :many
-- Deliberately omits a team predicate: FORCE RLS is the isolation mechanism
-- under proof. Keep this query bounded and out of production service paths.
SELECT id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id
FROM tasks
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');