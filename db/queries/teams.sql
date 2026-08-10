-- name: ListTeamsForUser :many
SELECT
    t.id,
    t.public_id,
    t.org_id,
    t.name,
    t.description,
    t.created_by_user,
    t.created_at,
    t.updated_at,
    tm.role AS my_role
FROM teams t
INNER JOIN team_members tm ON tm.team_id = t.id
WHERE t.org_id = $1
  AND tm.user_id = $2
ORDER BY t.name ASC;

-- name: ListTeamsByOrg :many
SELECT
    t.id,
    t.public_id,
    t.org_id,
    t.name,
    t.description,
    t.created_by_user,
    t.created_at,
    t.updated_at
FROM teams t
WHERE t.org_id = $1
ORDER BY t.name ASC;

-- name: GetTeamByPublicID :one
SELECT
    id,
    public_id,
    org_id,
    name,
    description,
    created_by_user,
    created_at,
    updated_at
FROM teams
WHERE public_id = $1;

-- name: GetTeamMembership :one
SELECT team_id, user_id, role, joined_at
FROM team_members
WHERE team_id = $1
  AND user_id = $2;

-- name: CreateTeam :one
INSERT INTO teams (org_id, name, description, created_by_user)
VALUES ($1, $2, $3, $4)
RETURNING id, public_id, org_id, name, description, created_by_user, created_at, updated_at;

-- name: AddTeamMember :one
INSERT INTO team_members (team_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING team_id, user_id, role, joined_at;

-- name: ListTeamMembers :many
SELECT
    tm.team_id,
    tm.user_id,
    tm.role,
    tm.joined_at,
    u.name AS user_name,
    u.email AS user_email,
    u.avatar_url AS user_avatar_url,
    u.active AS user_active
FROM team_members tm
INNER JOIN users u ON u.id = tm.user_id
WHERE tm.team_id = $1
ORDER BY
    CASE WHEN tm.role = 'TEAM_LEADER' THEN 0 ELSE 1 END,
    u.name ASC;

-- name: UpdateTeamMemberRole :one
UPDATE team_members
SET role = $3
WHERE team_id = $1
  AND user_id = $2
RETURNING team_id, user_id, role, joined_at;

-- name: RemoveTeamMember :exec
DELETE FROM team_members
WHERE team_id = $1
  AND user_id = $2;

-- name: CountTeamLeaders :one
SELECT COUNT(*)::bigint AS count
FROM team_members
WHERE team_id = $1
  AND role = 'TEAM_LEADER';
