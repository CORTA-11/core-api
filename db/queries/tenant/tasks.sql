-- name: GetTasks :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at, tasks.public_id, tasks.assignee_public_id, tasks.start_date, tasks.due_date, tasks.details
FROM tasks
ORDER BY tasks.created_at ASC, tasks.id ASC
LIMIT sqlc.arg('limit');

-- name: GetTasksAfter :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at, tasks.public_id, tasks.assignee_public_id, tasks.start_date, tasks.due_date, tasks.details
FROM tasks
WHERE (tasks.created_at, tasks.public_id) > (sqlc.arg('after_created_at'), sqlc.arg('after_public_id')::uuid)
ORDER BY tasks.created_at ASC, tasks.public_id ASC
LIMIT sqlc.arg('limit');

-- name: GetTasksBefore :many
SELECT tasks.id, tasks.team_id, tasks.description, tasks.status, tasks.created_at, tasks.updated_at, tasks.public_id, tasks.assignee_public_id, tasks.start_date, tasks.due_date, tasks.details
FROM tasks
WHERE (tasks.created_at, tasks.public_id) < (sqlc.arg('before_created_at'), sqlc.arg('before_public_id')::uuid)
ORDER BY tasks.created_at DESC, tasks.public_id DESC
LIMIT sqlc.arg('limit');

-- name: CreateTask :one
INSERT INTO tasks (team_id, description, status, assignee_public_id, start_date, due_date, details)
VALUES (NULLIF(current_setting('app.team_id', true), '')::BIGINT, $1, $2, $3, sqlc.narg('start_date'), sqlc.narg('due_date'), sqlc.arg('details'))
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id, start_date, due_date, details;

-- name: UpdateTask :one
UPDATE tasks
SET description = $2,
    status = $3,
    details = CASE WHEN sqlc.arg('set_details')::boolean THEN sqlc.arg('details')::text ELSE details END,
    assignee_public_id = CASE WHEN sqlc.arg('set_assignee')::boolean THEN sqlc.narg('assignee_public_id')::uuid ELSE assignee_public_id END,
    start_date = CASE WHEN sqlc.arg('set_start_date')::boolean THEN sqlc.narg('start_date')::timestamptz ELSE start_date END,
    due_date = CASE WHEN sqlc.arg('set_due_date')::boolean THEN sqlc.narg('due_date')::timestamptz ELSE due_date END,
    updated_at = NOW()
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id, start_date, due_date, details;

-- name: UnassignTask :one
UPDATE tasks
SET assignee_public_id = NULL,
    updated_at = NOW()
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id, start_date, due_date, details;

-- name: AssigneeIsMember :one
SELECT EXISTS (
    SELECT 1 FROM team_members
    WHERE team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
      AND user_public_id = $1
);

-- name: DeleteTask :one
DELETE FROM tasks
WHERE public_id = $1
RETURNING id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id, start_date, due_date, details;

-- name: IsolationProbeTasks :many
-- Deliberately omits a team predicate: FORCE RLS is the isolation mechanism
-- under proof. Keep this query bounded and out of production service paths.
SELECT id, team_id, description, status, created_at, updated_at, public_id, assignee_public_id, start_date, due_date, details
FROM tasks
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');