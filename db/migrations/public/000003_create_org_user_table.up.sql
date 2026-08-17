CREATE TABLE org_user (
    org_id BIGINT NOT NULL
        CONSTRAINT org_user_org_id_fk REFERENCES orgs (id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL
        CONSTRAINT org_user_user_id_fk REFERENCES users (id) ON DELETE CASCADE);
        