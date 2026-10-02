DROP FUNCTION IF EXISTS soft_delete_team(UUID);
DROP POLICY IF EXISTS teams_runtime_active ON teams;
ALTER TABLE teams DROP COLUMN deleted_at;
