DO $$
BEGIN
    RAISE EXCEPTION 'refusing to remove organization invitation security state; roll forward instead'
        USING ERRCODE = 'object_not_in_prerequisite_state';
END;
$$;
