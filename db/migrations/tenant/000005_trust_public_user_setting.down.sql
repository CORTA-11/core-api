DO $$
BEGIN
    RAISE EXCEPTION 'trusted public user RLS setting cannot be rolled back safely'
        USING ERRCODE = 'feature_not_supported';
END $$;
