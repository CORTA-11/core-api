DO $$
BEGIN
    EXECUTE format('ALTER SCHEMA %I OWNER TO synodus_owner', current_schema());
END $$;

ALTER TABLE teams OWNER TO synodus_owner;
ALTER TABLE team_members OWNER TO synodus_owner;
ALTER TABLE tasks OWNER TO synodus_owner;
ALTER TABLE schema_migrations OWNER TO synodus_owner;
ALTER SEQUENCE teams_id_seq OWNER TO synodus_owner;
ALTER SEQUENCE tasks_id_seq OWNER TO synodus_owner;

ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE teams FORCE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members FORCE ROW LEVEL SECURITY;
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks FORCE ROW LEVEL SECURITY;

CREATE FUNCTION synodus_app_user_public_id()
RETURNS UUID
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    setting TEXT;
    public_user_id UUID;
BEGIN
    setting := NULLIF(current_setting('app.user_id', true), '');
    IF setting IS NULL THEN
        RETURN NULL;
    END IF;
    BEGIN
        RETURN setting::UUID;
    EXCEPTION WHEN invalid_text_representation THEN
        BEGIN
            SELECT user_id INTO public_user_id
            FROM public.users
            WHERE id = setting::BIGINT AND deleted_at IS NULL;
            RETURN public_user_id;
        EXCEPTION WHEN invalid_text_representation OR numeric_value_out_of_range THEN
            RETURN NULL;
        END;
    END;
END;
$$;
ALTER FUNCTION synodus_app_user_public_id() OWNER TO synodus_owner;

CREATE FUNCTION synodus_has_team_membership(candidate_team_id BIGINT)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
    SELECT EXISTS (
        SELECT 1
        FROM team_members
        WHERE team_id = candidate_team_id
          AND user_public_id = synodus_app_user_public_id()
    );
$$;
ALTER FUNCTION synodus_has_team_membership(BIGINT) OWNER TO synodus_owner;

CREATE POLICY teams_owner_maintenance ON teams
    FOR ALL TO synodus_owner
    USING (true)
    WITH CHECK (true);
CREATE POLICY team_members_owner_maintenance ON team_members
    FOR ALL TO synodus_owner
    USING (true)
    WITH CHECK (true);
CREATE POLICY tasks_owner_maintenance ON tasks
    FOR ALL TO synodus_owner
    USING (true)
    WITH CHECK (true);

CREATE POLICY teams_runtime_select ON teams
    FOR SELECT TO synodus_runtime
    USING (NOT is_quarantine AND synodus_has_team_membership(id));

CREATE POLICY team_members_runtime_select ON team_members
    FOR SELECT TO synodus_runtime
    USING (synodus_has_team_membership(team_id));

CREATE POLICY tasks_runtime_access ON tasks
    FOR ALL TO synodus_runtime
    USING (
        team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
        AND synodus_has_team_membership(team_id)
    )
    WITH CHECK (
        team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
        AND synodus_has_team_membership(team_id)
    );

CREATE FUNCTION create_team_with_creator(team_name TEXT, team_slug TEXT)
RETURNS teams
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    creator UUID;
    created_team teams%ROWTYPE;
BEGIN
    creator := synodus_app_user_public_id();
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

    INSERT INTO teams (name, slug)
    VALUES (team_name, team_slug)
    RETURNING * INTO created_team;

    INSERT INTO team_members (team_id, user_public_id, role)
    VALUES (created_team.id, creator, 'team_admin');

    RETURN created_team;
END;
$$;
ALTER FUNCTION create_team_with_creator(TEXT, TEXT) OWNER TO synodus_owner;

REVOKE ALL ON teams, team_members, tasks, schema_migrations FROM PUBLIC;
REVOKE ALL ON teams_id_seq, tasks_id_seq FROM PUBLIC;
REVOKE ALL ON FUNCTION synodus_app_user_public_id() FROM PUBLIC;
REVOKE ALL ON FUNCTION synodus_has_team_membership(BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION create_team_with_creator(TEXT, TEXT) FROM PUBLIC;

REVOKE ALL ON teams, team_members, tasks, schema_migrations FROM synodus_runtime;
REVOKE ALL ON teams_id_seq, tasks_id_seq FROM synodus_runtime;
GRANT SELECT ON teams, team_members TO synodus_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON tasks TO synodus_runtime;
GRANT USAGE, SELECT ON tasks_id_seq TO synodus_runtime;
GRANT EXECUTE ON FUNCTION synodus_app_user_public_id() TO synodus_runtime;
GRANT EXECUTE ON FUNCTION synodus_has_team_membership(BIGINT) TO synodus_runtime;
GRANT EXECUTE ON FUNCTION create_team_with_creator(TEXT, TEXT) TO synodus_runtime;

DO $$
BEGIN
    EXECUTE format('REVOKE ALL ON SCHEMA %I FROM PUBLIC', current_schema());
    EXECUTE format('GRANT USAGE ON SCHEMA %I TO synodus_runtime', current_schema());
END $$;
