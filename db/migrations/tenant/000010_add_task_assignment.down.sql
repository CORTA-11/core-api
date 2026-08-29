DROP INDEX IF EXISTS tasks_assignee_team_idx;

ALTER TABLE tasks DROP COLUMN assignee_public_id;