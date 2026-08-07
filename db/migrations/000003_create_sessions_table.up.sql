CREATE TABLE sessions (
    id UUID
        DEFAULT gen_random_uuid()
        CONSTRAINT sessions_pk PRIMARY KEY,

    user_id BIGINT NOT NULL
        CONSTRAINT sessions_user_id_fkey REFERENCES users (id) ON DELETE CASCADE,

    refresh_token TEXT NOT NULL
        CONSTRAINT sessions_refresh_token_unique UNIQUE,

    is_blocked BOOLEAN NOT NULL DEFAULT false,

    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
