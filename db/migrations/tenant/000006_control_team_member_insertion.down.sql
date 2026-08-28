DO $$
BEGIN
    RAISE EXCEPTION 'controlled team membership insertion cannot be rolled back safely'
        USING ERRCODE = 'feature_not_supported';
END;
$$;
