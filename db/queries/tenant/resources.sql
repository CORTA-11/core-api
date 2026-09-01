-- name: ListResources :many
SELECT id, public_id, name, code, kind, location, enabled, availability, created_at, updated_at
FROM resources ORDER BY name, public_id
LIMIT sqlc.arg('limit');

-- name: GetResource :one
SELECT id, public_id, name, code, kind, location, enabled, availability, created_at, updated_at
FROM resources WHERE public_id = sqlc.arg('public_id');

-- name: CreateResource :one
INSERT INTO resources (name, code, kind, location, enabled, availability)
VALUES (sqlc.arg('name'), sqlc.arg('code'), sqlc.arg('kind'), sqlc.arg('location'),
        sqlc.arg('enabled'), sqlc.arg('availability'))
RETURNING id, public_id, name, code, kind, location, enabled, availability, created_at, updated_at;

-- name: UpdateResource :one
UPDATE resources SET name = sqlc.arg('name'), code = sqlc.arg('code'), kind = sqlc.arg('kind'),
    location = sqlc.arg('location'), enabled = sqlc.arg('enabled'),
    availability = sqlc.arg('availability'), updated_at = now()
WHERE public_id = sqlc.arg('public_id')
RETURNING id, public_id, name, code, kind, location, enabled, availability, created_at, updated_at;

-- name: DeleteResource :execrows
DELETE FROM resources WHERE public_id = sqlc.arg('public_id');

-- name: CreateResourceRequest :one
INSERT INTO resource_requests (resource_id, team_id, requested_by, start_time, end_time, purpose)
SELECT resource.id, team.id, synodus_app_user_public_id(), sqlc.arg('start_time'),
       sqlc.arg('end_time'), sqlc.arg('purpose')
FROM resources AS resource
JOIN teams AS team ON team.id = nullif(current_setting('app.team_id', true), '')::bigint
WHERE resource.public_id = sqlc.arg('resource_public_id') AND team.public_id = sqlc.arg('team_public_id')
RETURNING id, public_id, resource_id, team_id, requested_by, start_time, end_time,
          purpose, status, created_at, decided_at;

-- name: ListBookings :many
SELECT request.public_id, resource.public_id AS resource_public_id, resource.name AS resource_name,
       request.start_time, request.end_time,
       coalesce(synodus_current_organization_role() IN ('owner', 'administrator') OR team_member.user_public_id IS NOT NULL, false)::boolean AS details_visible,
       coalesce(CASE WHEN synodus_current_organization_role() IN ('owner', 'administrator') OR team_member.user_public_id IS NOT NULL THEN team.public_id END, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS team_public_id,
       coalesce(CASE WHEN synodus_current_organization_role() IN ('owner', 'administrator') OR team_member.user_public_id IS NOT NULL THEN team.name END, '')::text AS team_name,
       coalesce(CASE WHEN synodus_current_organization_role() IN ('owner', 'administrator') OR team_member.user_public_id IS NOT NULL THEN synodus_user_display_name(request.requested_by) END, '')::text AS requested_by_name,
       coalesce(CASE WHEN synodus_current_organization_role() IN ('owner', 'administrator') OR team_member.user_public_id IS NOT NULL THEN request.purpose END, '')::text AS purpose
FROM resource_requests AS request
JOIN resources AS resource ON resource.id = request.resource_id
JOIN teams AS team ON team.id = request.team_id
LEFT JOIN team_members AS team_member ON team_member.team_id = request.team_id
    AND team_member.user_public_id = synodus_app_user_public_id()
WHERE request.status = 'approved'
ORDER BY request.start_time, request.public_id
LIMIT sqlc.arg('limit');

-- name: ListResourceRequests :many
SELECT request.public_id, resource.public_id AS resource_public_id, resource.name AS resource_name,
       team.public_id AS team_public_id, team.name AS team_name, request.requested_by,
       synodus_user_display_name(request.requested_by) AS requested_by_name, request.start_time, request.end_time,
       request.purpose, request.status, request.created_at, request.decided_at
FROM resource_requests AS request
JOIN resources AS resource ON resource.id = request.resource_id
JOIN teams AS team ON team.id = request.team_id
LEFT JOIN team_members AS team_member ON team_member.team_id = request.team_id
    AND team_member.user_public_id = synodus_app_user_public_id()
WHERE synodus_current_organization_role() IN ('owner', 'administrator') OR team_member.user_public_id IS NOT NULL
ORDER BY request.created_at DESC, request.public_id
LIMIT sqlc.arg('limit');

-- name: GetResourceRequest :one
SELECT request.id, request.public_id, request.resource_id, request.team_id, request.requested_by,
       request.start_time, request.end_time, request.purpose, request.status, request.created_at,
       request.decided_at, resource.public_id AS resource_public_id, resource.name AS resource_name,
       resource.enabled, resource.availability, team.public_id AS team_public_id, team.name AS team_name,
       synodus_user_display_name(request.requested_by) AS requested_by_name
FROM resource_requests AS request
JOIN resources AS resource ON resource.id = request.resource_id
JOIN teams AS team ON team.id = request.team_id
WHERE request.public_id = sqlc.arg('public_id');

-- name: LockResourceForDecision :one
SELECT resource.id, resource.enabled, resource.availability
FROM resources AS resource
JOIN resource_requests AS request ON request.resource_id = resource.id
WHERE request.public_id = sqlc.arg('request_public_id')
FOR UPDATE OF resource;

-- name: ApprovedResourceRequestOverlap :one
SELECT EXISTS (
    SELECT 1 FROM resource_requests
    WHERE resource_id = sqlc.arg('resource_id') AND status = 'approved'
      AND start_time < sqlc.arg('end_time') AND sqlc.arg('start_time') < end_time
      AND public_id <> sqlc.arg('request_public_id')
) AS overlaps;

-- name: DecideResourceRequest :one
UPDATE resource_requests
SET status = sqlc.arg('status'), decided_at = now()
WHERE public_id = sqlc.arg('public_id') AND status = 'pending'
RETURNING id, public_id, resource_id, team_id, requested_by, start_time, end_time,
          purpose, status, created_at, decided_at;
