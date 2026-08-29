CREATE TABLE public.user_public_keys (
    user_id UUID PRIMARY KEY REFERENCES public.users(user_id) ON DELETE CASCADE,
    public_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE public.user_public_keys OWNER TO synodus_owner;
REVOKE ALL ON public.user_public_keys FROM synodus_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.user_public_keys TO synodus_runtime;
