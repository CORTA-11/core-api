LOCK TABLE public.org_user IN ACCESS EXCLUSIVE MODE;

-- Legacy rows carry no distinguishing authorization data. Keep one arbitrary
-- physical row per pair, then grant every surviving membership the same
-- least-privileged role below. No ordering can safely identify an owner.
DELETE FROM public.org_user AS duplicate
USING public.org_user AS retained
WHERE duplicate.org_id = retained.org_id
  AND duplicate.user_id = retained.user_id
  AND duplicate.ctid > retained.ctid;

ALTER TABLE public.org_user
    ADD COLUMN role TEXT NOT NULL DEFAULT 'member',
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD CONSTRAINT org_user_role_check
        CHECK (role IN ('owner', 'administrator', 'member')),
    ADD CONSTRAINT org_user_timestamps_check
        CHECK (updated_at >= created_at),
    ADD CONSTRAINT org_user_pk PRIMARY KEY (org_id, user_id);

CREATE INDEX org_user_user_org_idx ON public.org_user (user_id, org_id);
