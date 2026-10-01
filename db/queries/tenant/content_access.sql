-- name: ListContentAccess :many
SELECT o.kind, o.resource_id, o.creator_id,
       synodus_can_access_content(o.kind, o.resource_id)::boolean AS can_access
FROM content_owners o
ORDER BY o.kind, o.resource_id
LIMIT sqlc.arg('limit');

-- name: ListContentAccessRequests :many
SELECT r.public_id, r.kind, r.resource_id, r.requested_by, r.status, r.updated_at,
       coalesce(synodus_user_display_name(r.requested_by), '')::text AS requester_name
FROM content_access_requests r
ORDER BY r.updated_at DESC, r.public_id
LIMIT sqlc.arg('limit');

-- name: RequestContentAccess :one
INSERT INTO content_access_requests (kind, resource_id, requested_by)
SELECT o.kind, o.resource_id, synodus_app_user_public_id()
FROM content_owners o
WHERE o.kind = sqlc.arg('kind') AND o.resource_id = sqlc.arg('resource_id')
  AND o.creator_id <> synodus_app_user_public_id()
ON CONFLICT (kind, resource_id, requested_by) DO UPDATE
SET status = 'pending', updated_at = now()
WHERE content_access_requests.status = 'denied'
RETURNING public_id, kind, resource_id, requested_by, status, updated_at;

-- name: DecideContentAccess :one
UPDATE content_access_requests r
SET status = sqlc.arg('status'), updated_at = now()
FROM content_owners o
WHERE r.public_id = sqlc.arg('public_id') AND r.status = 'pending'
  AND o.kind = r.kind AND o.resource_id = r.resource_id
  AND o.creator_id = synodus_app_user_public_id()
  AND (sqlc.arg('status')::text = 'denied' OR EXISTS (
      SELECT 1 FROM team_members m WHERE m.team_id = o.team_id AND m.user_public_id = r.requested_by))
RETURNING r.public_id, r.kind, r.resource_id, r.requested_by, r.status, r.updated_at;

-- name: GrantContentAccess :one
INSERT INTO content_access_requests (kind, resource_id, requested_by, status)
SELECT o.kind, o.resource_id, sqlc.arg('requested_by')::uuid, 'granted'
FROM content_owners o
WHERE o.kind = sqlc.arg('kind') AND o.resource_id = sqlc.arg('resource_id')
  AND o.creator_id = synodus_app_user_public_id()
  AND EXISTS (SELECT 1 FROM team_members m WHERE m.team_id = o.team_id AND m.user_public_id = sqlc.arg('requested_by')::uuid)
ON CONFLICT (kind, resource_id, requested_by) DO UPDATE
SET status = 'granted', updated_at = now()
RETURNING public_id, kind, resource_id, requested_by, status, updated_at;
