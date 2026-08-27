DO $$
BEGIN
    RAISE EXCEPTION 'refusing to roll back forward-only migration 000008; sessions contain security state';
END $$;
