ALTER TABLE teams ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

CREATE OR REPLACE FUNCTION update_team(team_name TEXT, team_slug TEXT, team_description TEXT)
RETURNS teams
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    bound_team BIGINT;
    caller UUID;
    updated_team teams%ROWTYPE;
BEGIN
    bound_team := NULLIF(current_setting('app.team_id', true), '')::BIGINT;
    caller := synodus_app_user_public_id();

    IF bound_team IS NULL OR caller IS NULL OR NOT (
        EXISTS (
            SELECT 1 FROM team_members
            WHERE team_id = bound_team
              AND user_public_id = caller
              AND role = 'team_admin'
        )
        OR EXISTS (
            SELECT 1
            FROM public.users AS app_user
            JOIN public.org_user AS organization_member
              ON organization_member.user_id = app_user.id
            JOIN public.orgs AS organization
              ON organization.id = organization_member.org_id
            WHERE app_user.user_id = caller
              AND app_user.deleted_at IS NULL
              AND organization.schema_name = current_schema()
              AND organization.deleted_at IS NULL
              AND organization.lifecycle_state = 'active'
              AND organization_member.role IN ('owner', 'admin')
        )
    ) THEN
        RAISE EXCEPTION 'team unavailable' USING ERRCODE = 'insufficient_privilege';
    END IF;

    UPDATE teams
    SET name = COALESCE(NULLIF(TRIM(team_name), ''), name),
        slug = CASE
            WHEN team_name IS NOT NULL AND TRIM(team_name) <> '' AND team_slug IS NOT NULL AND TRIM(team_slug) <> ''
            THEN TRIM(team_slug)
            ELSE slug
        END,
        description = COALESCE(team_description, description),
        updated_at = now()
    WHERE id = bound_team
      AND NOT is_quarantine
    RETURNING * INTO updated_team;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'team not found' USING ERRCODE = 'no_data_found';
    END IF;

    RETURN updated_team;
END;
$$;

ALTER FUNCTION update_team(TEXT, TEXT, TEXT) OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION update_team(TEXT, TEXT, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION update_team(TEXT, TEXT, TEXT) TO synodus_runtime;
