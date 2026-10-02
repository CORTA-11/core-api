ALTER TABLE teams ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE POLICY teams_runtime_active ON teams AS RESTRICTIVE
    FOR SELECT TO synodus_runtime USING (deleted_at IS NULL);

CREATE FUNCTION soft_delete_team(candidate UUID)
RETURNS UUID
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    target BIGINT;
    caller UUID := synodus_app_user_public_id();
BEGIN
    SELECT id INTO target FROM teams
    WHERE public_id = candidate AND deleted_at IS NULL AND NOT is_quarantine
    FOR UPDATE;
    IF target IS NULL THEN
        RAISE EXCEPTION 'team not found' USING ERRCODE = 'no_data_found';
    END IF;
    IF caller IS NULL OR NOT (
        COALESCE(synodus_current_organization_role() IN ('owner', 'administrator'), FALSE)
        OR EXISTS (
            SELECT 1 FROM team_members
            WHERE team_id = target AND user_public_id = caller AND role = 'team_admin'
        )
    ) THEN
        RAISE EXCEPTION 'team unavailable' USING ERRCODE = 'insufficient_privilege';
    END IF;
    UPDATE teams SET deleted_at = now(), updated_at = now() WHERE id = target;
    RETURN candidate;
END;
$$;
ALTER FUNCTION soft_delete_team(UUID) OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION soft_delete_team(UUID) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION soft_delete_team(UUID) TO synodus_runtime;
