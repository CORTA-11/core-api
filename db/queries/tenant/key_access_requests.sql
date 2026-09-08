-- name: CreateKeyAccessRequest :one
INSERT INTO team_key_access_requests (team_id, requested_by)
SELECT team.id, synodus_app_user_public_id()
FROM teams AS team
WHERE team.id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
RETURNING id, public_id, team_id, requested_by, status, created_at, decided_by, decided_at,
          (SELECT public_id FROM teams WHERE id = team_id) AS team_public_id,
          (SELECT name FROM teams WHERE id = team_id) AS team_name,
          synodus_user_display_name(requested_by) AS requested_by_name;

-- name: GetKeyAccessRequest :one
SELECT id, public_id, team_id, requested_by, status, created_at, decided_by, decided_at,
       (SELECT public_id FROM teams WHERE id = team_id) AS team_public_id,
       (SELECT name FROM teams WHERE id = team_id) AS team_name,
       synodus_user_display_name(requested_by) AS requested_by_name
FROM team_key_access_requests
WHERE team_key_access_requests.public_id = sqlc.arg('public_id');

-- name: ListKeyAccessRequests :many
SELECT request.public_id, request.requested_by,
       synodus_user_display_name(request.requested_by) AS requested_by_name,
       request.status, request.created_at, request.decided_at,
       request.decided_by, coalesce(synodus_user_display_name(request.decided_by), '')::text AS decided_by_name,
       team.public_id AS team_public_id, team.name AS team_name
FROM team_key_access_requests AS request
JOIN teams AS team ON team.id = request.team_id
ORDER BY request.created_at DESC, request.public_id
LIMIT sqlc.arg('limit');

-- name: DecideKeyAccessRequest :one
UPDATE team_key_access_requests
SET status = sqlc.arg('status'), decided_by = synodus_app_user_public_id(), decided_at = now()
WHERE team_key_access_requests.public_id = sqlc.arg('public_id') AND status = 'pending'
RETURNING id, public_id, team_id, requested_by, status, created_at, decided_by, decided_at,
          (SELECT public_id FROM teams WHERE id = team_id) AS team_public_id,
          (SELECT name FROM teams WHERE id = team_id) AS team_name,
          synodus_user_display_name(requested_by) AS requested_by_name,
          synodus_user_display_name(decided_by) AS decided_by_name;