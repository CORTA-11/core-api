-- name: GetTeams :many
SELECT id, name, slug, created_at, updated_at, public_id, is_quarantine
FROM teams
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: GetTeamsAfter :many
SELECT id, name, slug, created_at, updated_at, public_id, is_quarantine
FROM teams
WHERE NOT is_quarantine
  AND (created_at, public_id) > (sqlc.arg('after_created_at'), sqlc.arg('after_public_id')::uuid)
ORDER BY created_at ASC, public_id ASC
LIMIT sqlc.arg('limit');

-- name: GetTeamsBefore :many
SELECT id, name, slug, created_at, updated_at, public_id, is_quarantine
FROM teams
WHERE NOT is_quarantine
  AND (created_at, public_id) < (sqlc.arg('before_created_at'), sqlc.arg('before_public_id')::uuid)
ORDER BY created_at DESC, public_id DESC
LIMIT sqlc.arg('limit');

-- name: CreateTeam :one
INSERT INTO teams (name, slug)
VALUES ($1, $2)
RETURNING id, name, slug, created_at, updated_at, public_id, is_quarantine;

-- name: CreateTeamWithCreator :one
SELECT id, name, slug, created_at, updated_at, public_id, is_quarantine
FROM create_team_with_creator(sqlc.arg('name'), sqlc.arg('slug'), sqlc.arg('leader_email'));

-- name: ResolveTeamContext :one
SELECT teams.id, teams.public_id
FROM teams
JOIN team_members
  ON team_members.team_id = teams.id
WHERE teams.public_id = sqlc.arg('public_id')
  AND team_members.user_public_id = sqlc.arg('user_public_id')
  AND NOT teams.is_quarantine;

-- name: RevalidateTeamAuthorization :one
SELECT teams.public_id, team_members.role
FROM teams
JOIN team_members ON team_members.team_id = teams.id
WHERE teams.id = nullif(current_setting('app.team_id', true), '')::bigint
  AND teams.public_id = sqlc.arg('team_public_id')
  AND team_members.user_public_id = sqlc.arg('user_public_id')
  AND NOT teams.is_quarantine;

-- name: GetCurrentTeamRole :one
SELECT role FROM team_members
WHERE team_id = sqlc.arg('team_id')
  AND user_public_id = synodus_app_user_public_id();

-- name: GetBoundTeamID :one
SELECT NULLIF(current_setting('app.team_id', true), '')::BIGINT AS team_id;

-- name: ListTeamMembersAfter :many
SELECT user_public_id, role, created_at
FROM team_members
WHERE team_id = nullif(current_setting('app.team_id', true), '')::bigint
ORDER BY created_at, user_public_id
LIMIT sqlc.arg('limit');

-- name: AddTeamContributor :one
SELECT team_id, user_public_id, role, created_at, updated_at
FROM add_team_contributor(sqlc.arg('email'));
