-- Roles are database-cluster objects, so this migration must initially be run
-- by the existing database administrator. Existing roles are adopted only when
-- their attributes are inside the exact non-privileged allowlist below.
DO $$
DECLARE
    role_record RECORD;
BEGIN
    FOR role_record IN
        SELECT * FROM (VALUES
            ('synodus_owner', false),
            ('synodus_migrator', true),
            ('synodus_provisioner', true),
            ('synodus_runtime', true)
        ) AS desired(role_name, can_login)
    LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_record.role_name) THEN
            IF EXISTS (
                SELECT 1 FROM pg_roles
                WHERE rolname = role_record.role_name
                  AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
            ) THEN
                RAISE EXCEPTION 'refusing to adopt privileged database role %', role_record.role_name
                    USING ERRCODE = 'invalid_authorization_specification';
            END IF;
        ELSE
            EXECUTE format('CREATE ROLE %I', role_record.role_name);
        END IF;
    END LOOP;
END $$;

ALTER ROLE synodus_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE synodus_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE synodus_provisioner LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE synodus_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;

GRANT synodus_owner TO synodus_migrator WITH INHERIT FALSE, SET TRUE;
GRANT synodus_owner TO synodus_provisioner WITH INHERIT FALSE, SET TRUE;
ALTER ROLE synodus_migrator SET ROLE TO synodus_owner;
ALTER ROLE synodus_provisioner SET ROLE TO synodus_owner;

DO $$
BEGIN
    EXECUTE format('REVOKE ALL ON DATABASE %I FROM synodus_runtime', current_database());
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO synodus_runtime', current_database());
    EXECUTE format('GRANT CONNECT, CREATE, TEMPORARY ON DATABASE %I TO synodus_owner', current_database());
END $$;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO synodus_owner;
GRANT USAGE ON SCHEMA public TO synodus_runtime;

ALTER TABLE public.orgs OWNER TO synodus_owner;
ALTER TABLE public.users OWNER TO synodus_owner;
ALTER TABLE public.org_user OWNER TO synodus_owner;
ALTER TABLE public.schema_migrations OWNER TO synodus_owner;
ALTER SEQUENCE public.orgs_id_seq OWNER TO synodus_owner;
ALTER SEQUENCE public.users_id_seq OWNER TO synodus_owner;

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM synodus_runtime;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM synodus_runtime;
REVOKE ALL ON public.schema_migrations FROM synodus_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.orgs, public.users, public.org_user TO synodus_runtime;
GRANT USAGE, SELECT ON public.orgs_id_seq, public.users_id_seq TO synodus_runtime;
