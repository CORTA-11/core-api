DO $$
BEGIN
    RAISE EXCEPTION 'forced team row security cannot be rolled back safely'
        USING ERRCODE = 'feature_not_supported';
END $$;
