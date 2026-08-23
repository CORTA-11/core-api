CREATE OR REPLACE FUNCTION synodus_app_user_public_id()
RETURNS UUID
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
    SELECT NULLIF(current_setting('app.user_id', true), '')::UUID;
$$;
ALTER FUNCTION synodus_app_user_public_id() OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION synodus_app_user_public_id() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION synodus_app_user_public_id() TO synodus_runtime;
