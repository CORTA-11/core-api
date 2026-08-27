-- name: GetTeams :many
SELECT id, name, slug, created_at, updated_at, public_id, is_quarantine
FROM teams
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: CreateTeam :one
INSERT INTO teams (name, slug)
VALUES ($1, $2)
RETURNING id, name, slug, created_at, updated_at, public_id, is_quarantine;

-- name: CreateTeamWithCreator :one
SELECT id, name, slug, created_at, updated_at, public_id, is_quarantine
FROM create_team_with_creator(sqlc.arg('name'), sqlc.arg('slug'));

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
