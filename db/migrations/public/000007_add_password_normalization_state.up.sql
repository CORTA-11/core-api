ALTER TABLE public.users
    ADD COLUMN password_normalization TEXT;

UPDATE public.users
SET password_normalization = 'legacy_raw'
WHERE password_normalization IS NULL;

ALTER TABLE public.users
    ALTER COLUMN password_normalization SET NOT NULL,
    ALTER COLUMN password_normalization SET DEFAULT 'legacy_raw',
    ADD CONSTRAINT users_password_normalization_closed CHECK (
        password_normalization IN ('legacy_raw', 'nfc_v1')
    );
