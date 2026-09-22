CREATE TABLE public.user_device_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES public.users(user_id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    platform VARCHAR(32) NOT NULL DEFAULT 'android',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_device_tokens_user_id ON public.user_device_tokens (user_id);

ALTER TABLE public.user_device_tokens OWNER TO synodus_owner;
REVOKE ALL ON public.user_device_tokens FROM synodus_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.user_device_tokens TO synodus_runtime;
