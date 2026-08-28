DO $$
BEGIN
    RAISE EXCEPTION 'refusing to remove invitation locking authorization; roll forward instead'
        USING ERRCODE = 'object_not_in_prerequisite_state';
END;
$$;
