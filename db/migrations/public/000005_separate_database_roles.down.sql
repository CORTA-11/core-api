DO $$
BEGIN
    RAISE EXCEPTION 'database role separation cannot be rolled back safely'
        USING ERRCODE = 'feature_not_supported';
END $$;
