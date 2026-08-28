CREATE FUNCTION add_team_contributor(target_email TEXT)
RETURNS team_members
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    bound_team BIGINT;
    target_user UUID;
    added team_members%ROWTYPE;
BEGIN
    bound_team := NULLIF(current_setting('app.team_id', true), '')::BIGINT;
    IF bound_team IS NULL OR NOT EXISTS (
        SELECT 1 FROM team_members
        WHERE team_id = bound_team
          AND user_public_id = synodus_app_user_public_id()
          AND role = 'team_admin'
    ) THEN
        RAISE EXCEPTION 'team unavailable' USING ERRCODE = 'insufficient_privilege';
    END IF;

    SELECT app_user.user_id INTO target_user
    FROM public.users AS app_user
    JOIN public.org_user AS membership ON membership.user_id = app_user.id
    JOIN public.orgs AS organization ON organization.id = membership.org_id
    WHERE app_user.email_canonical = public.canonical_email_key(target_email)
      AND app_user.deleted_at IS NULL
      AND organization.schema_name = current_schema()
      AND organization.deleted_at IS NULL
      AND organization.lifecycle_state = 'active';
    IF target_user IS NULL THEN
        RAISE EXCEPTION 'team unavailable' USING ERRCODE = 'no_data_found';
    END IF;

    INSERT INTO team_members (team_id, user_public_id, role)
    VALUES (bound_team, target_user, 'contributor')
    RETURNING * INTO added;
    RETURN added;
END;
$$;
ALTER FUNCTION add_team_contributor(TEXT) OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION add_team_contributor(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION add_team_contributor(TEXT) TO synodus_runtime;

CREATE FUNCTION list_bound_team_members()
RETURNS TABLE (user_public_id UUID, display_name TEXT, email TEXT, role TEXT, joined_at TIMESTAMPTZ)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
    SELECT member.user_public_id, app_user.display_name, app_user.email, member.role, member.created_at
    FROM team_members AS member
    JOIN public.users AS app_user ON app_user.user_id = member.user_public_id
    WHERE member.team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
      AND app_user.deleted_at IS NULL
$$;
ALTER FUNCTION list_bound_team_members() OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION list_bound_team_members() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION list_bound_team_members() TO synodus_runtime;
