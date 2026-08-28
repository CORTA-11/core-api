CREATE OR REPLACE FUNCTION create_team_with_creator(team_name TEXT, team_slug TEXT, leader_email TEXT)
RETURNS teams
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    creator UUID;
    leader_id UUID;
    created_team teams%ROWTYPE;
BEGIN
    creator := synodus_app_user_public_id();
    -- Validate creator has active org membership
    IF creator IS NULL OR NOT EXISTS (
        SELECT 1
        FROM public.users AS app_user
        JOIN public.org_user AS organization_member
          ON organization_member.user_id = app_user.id
        JOIN public.orgs AS organization
          ON organization.id = organization_member.org_id
        WHERE app_user.user_id = creator
          AND app_user.deleted_at IS NULL
          AND organization.schema_name = current_schema()
          AND organization.deleted_at IS NULL
          AND organization.lifecycle_state = 'active'
    ) THEN
        RAISE EXCEPTION 'organization unavailable'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    -- Resolve the leader email to their UUID
    SELECT app_user.user_id INTO leader_id
    FROM public.users AS app_user
    JOIN public.org_user AS membership ON membership.user_id = app_user.id
    JOIN public.orgs AS organization ON organization.id = membership.org_id
    WHERE app_user.email_canonical = public.canonical_email_key(leader_email)
      AND app_user.deleted_at IS NULL
      AND organization.schema_name = current_schema()
      AND organization.deleted_at IS NULL
      AND organization.lifecycle_state = 'active';

    IF leader_id IS NULL THEN
        RAISE EXCEPTION 'team leader not found' USING ERRCODE = 'no_data_found';
    END IF;

    -- Insert the team
    INSERT INTO teams (name, slug)
    VALUES (team_name, team_slug)
    RETURNING * INTO created_team;

    -- Insert the selected leader as team_admin (leader)
    INSERT INTO team_members (team_id, user_public_id, role)
    VALUES (created_team.id, leader_id, 'team_admin');

    RETURN created_team;
END;
$$;

ALTER FUNCTION create_team_with_creator(TEXT, TEXT, TEXT) OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION create_team_with_creator(TEXT, TEXT, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION create_team_with_creator(TEXT, TEXT, TEXT) TO synodus_runtime;
