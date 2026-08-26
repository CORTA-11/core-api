DO $$
BEGIN
    RAISE EXCEPTION 'refusing to remove additive canonical email identity state; roll forward instead'
        USING ERRCODE = 'object_not_in_prerequisite_state';
END;
$$;
