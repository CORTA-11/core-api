DO $$
BEGIN
    RAISE EXCEPTION 'refusing to remove organization authorization state; roll forward instead'
        USING ERRCODE = 'object_not_in_prerequisite_state';
END;
$$;
