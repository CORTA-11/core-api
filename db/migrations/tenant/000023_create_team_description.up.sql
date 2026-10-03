-- Preserve the existing creation checks and leader assignment, while storing
-- the description in the same transaction as the new team.
CREATE FUNCTION create_team_with_creator(team_name TEXT, team_slug TEXT, leader_email TEXT, team_description TEXT)
RETURNS teams
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    created_team teams%ROWTYPE;
BEGIN
    SELECT * INTO created_team
    FROM create_team_with_creator(team_name, team_slug, leader_email);

    UPDATE teams
    SET description = COALESCE(team_description, '')
    WHERE id = created_team.id
    RETURNING * INTO created_team;

    RETURN created_team;
END;
$$;

ALTER FUNCTION create_team_with_creator(TEXT, TEXT, TEXT, TEXT) OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION create_team_with_creator(TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION create_team_with_creator(TEXT, TEXT, TEXT, TEXT) TO synodus_runtime;
