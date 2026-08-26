CREATE OR REPLACE FUNCTION public.canonical_email_display(value TEXT)
RETURNS TEXT
LANGUAGE plpgsql
IMMUTABLE
STRICT
PARALLEL SAFE
AS $$
DECLARE
    normalized TEXT;
    code_point INTEGER;
    position INTEGER;
BEGIN
    normalized := normalize(btrim(value, ' '), NFC);
    IF normalized = '' OR octet_length(normalized) > 254 THEN
        RETURN NULL;
    END IF;

    FOR position IN 1..char_length(normalized) LOOP
        code_point := ascii(substr(normalized, position, 1));
        IF code_point BETWEEN 0 AND 31 OR code_point BETWEEN 127 AND 159 THEN
            RETURN NULL;
        END IF;
    END LOOP;
    RETURN normalized;
END;
$$;

CREATE OR REPLACE FUNCTION public.canonical_email_key(value TEXT)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
RETURN normalize(
    casefold(public.canonical_email_display(value) COLLATE pg_catalog.pg_unicode_fast),
    NFC
);

ALTER FUNCTION public.canonical_email_display(TEXT) OWNER TO synodus_owner;
ALTER FUNCTION public.canonical_email_key(TEXT) OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION public.canonical_email_display(TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.canonical_email_key(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.canonical_email_display(TEXT) TO synodus_runtime, synodus_migrator, synodus_provisioner;
GRANT EXECUTE ON FUNCTION public.canonical_email_key(TEXT) TO synodus_runtime, synodus_migrator, synodus_provisioner;

LOCK TABLE public.users IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    invalid_user_ids TEXT;
BEGIN
    SELECT string_agg(user_id::TEXT, ', ' ORDER BY user_id::TEXT)
    INTO invalid_user_ids
    FROM (
        SELECT user_id
        FROM public.users
        WHERE public.canonical_email_key(email) IS NULL
        ORDER BY user_id
        LIMIT 10
    ) AS invalid_users;

    IF invalid_user_ids IS NOT NULL THEN
        RAISE EXCEPTION 'invalid canonical email for user public ids: %', invalid_user_ids
            USING ERRCODE = 'check_violation';
    END IF;
END;
$$;

DO $$
DECLARE
    colliding_user_ids TEXT;
BEGIN
    SELECT string_agg(user_id::TEXT, ', ' ORDER BY user_id::TEXT)
    INTO colliding_user_ids
    FROM (
        SELECT app_user.user_id
        FROM public.users AS app_user
        JOIN (
            SELECT public.canonical_email_key(email) AS email_key
            FROM public.users
            GROUP BY public.canonical_email_key(email)
            HAVING count(*) > 1
        ) AS collision
          ON collision.email_key = public.canonical_email_key(app_user.email)
        ORDER BY app_user.user_id
        LIMIT 10
    ) AS colliding_users;

    IF colliding_user_ids IS NOT NULL THEN
        RAISE EXCEPTION 'canonical email collision for user public ids: %', colliding_user_ids
            USING ERRCODE = 'unique_violation';
    END IF;
END;
$$;

UPDATE public.users
SET email = public.canonical_email_display(email)
WHERE email IS DISTINCT FROM public.canonical_email_display(email);

ALTER TABLE public.users
    DROP CONSTRAINT users_email_unique,
    ADD CONSTRAINT users_email_display_valid CHECK (
        email = public.canonical_email_display(email)
    ),
    ADD COLUMN email_canonical TEXT
        GENERATED ALWAYS AS (public.canonical_email_key(email)) STORED NOT NULL,
    ADD CONSTRAINT users_email_canonical_unique UNIQUE (email_canonical);
