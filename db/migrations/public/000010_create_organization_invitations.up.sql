CREATE TABLE public.organization_invitations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id UUID NOT NULL DEFAULT gen_random_uuid(),
    org_id BIGINT NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    email_hmac BYTEA NOT NULL CHECK (octet_length(email_hmac) = 32),
    token_hash BYTEA NOT NULL CHECK (octet_length(token_hash) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT organization_invitations_public_id_unique UNIQUE (public_id),
    CONSTRAINT organization_invitations_org_email_unique UNIQUE (org_id, email_hmac),
    CONSTRAINT organization_invitations_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT organization_invitations_fixed_expiry CHECK (expires_at = created_at + interval '7 days')
);

ALTER TABLE public.organization_invitations OWNER TO synodus_owner;
REVOKE ALL ON public.organization_invitations FROM PUBLIC;
GRANT SELECT, INSERT, DELETE ON public.organization_invitations TO synodus_runtime;
GRANT USAGE, SELECT ON SEQUENCE public.organization_invitations_id_seq TO synodus_runtime;
