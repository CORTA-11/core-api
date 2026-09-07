ALTER TABLE public.user_public_keys
    ADD COLUMN encrypted_private_key TEXT,
    ADD COLUMN kek_salt TEXT,
    ADD COLUMN kek_iterations INTEGER,
    ADD COLUMN kek_algorithm VARCHAR(32) NOT NULL DEFAULT 'pbkdf2-sha256',
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();