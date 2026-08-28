DO $$
BEGIN
    RAISE EXCEPTION 'create team update cannot be rolled back safely'
        USING ERRCODE = 'feature_not_supported';
END;
$$;
