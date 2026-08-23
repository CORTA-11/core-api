DO $$
BEGIN
    RAISE EXCEPTION 'team identity and quarantine migration cannot be rolled back safely'
        USING ERRCODE = 'feature_not_supported';
END $$;
