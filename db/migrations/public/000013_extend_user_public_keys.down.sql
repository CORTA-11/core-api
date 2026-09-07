ALTER TABLE public.user_public_keys
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS kek_algorithm,
    DROP COLUMN IF EXISTS kek_iterations,
    DROP COLUMN IF EXISTS kek_salt,
    DROP COLUMN IF EXISTS encrypted_private_key;