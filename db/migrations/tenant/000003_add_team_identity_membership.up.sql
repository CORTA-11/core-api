ALTER TABLE teams
    ADD COLUMN public_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN is_quarantine BOOLEAN NOT NULL DEFAULT false,
    ADD CONSTRAINT teams_public_id_unique UNIQUE (public_id);

CREATE UNIQUE INDEX teams_single_quarantine_idx
    ON teams ((is_quarantine))
    WHERE is_quarantine;

ALTER TABLE tasks
    ADD COLUMN public_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD CONSTRAINT tasks_public_id_unique UNIQUE (public_id);

CREATE TABLE team_members (
    team_id BIGINT NOT NULL
        CONSTRAINT team_members_team_fk
        REFERENCES teams(id)
        ON DELETE CASCADE,
    user_public_id UUID NOT NULL
        CONSTRAINT team_members_user_public_fk
        REFERENCES public.users(user_id)
        ON DELETE CASCADE,
    role TEXT NOT NULL
        CONSTRAINT team_members_role_check
        CHECK (role IN ('team_admin', 'research_lead', 'researcher', 'contributor', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT team_members_pk PRIMARY KEY (team_id, user_public_id)
);

CREATE INDEX team_members_user_team_idx
    ON team_members (user_public_id, team_id);

INSERT INTO teams (name, slug, is_quarantine)
SELECT 'Synodus inaccessible task quarantine', '__synodus_task_quarantine__', true
WHERE EXISTS (SELECT 1 FROM tasks WHERE team_id IS NULL);

UPDATE tasks
SET team_id = (SELECT id FROM teams WHERE is_quarantine)
WHERE team_id IS NULL;

ALTER TABLE tasks
    ALTER COLUMN team_id SET NOT NULL;

CREATE INDEX tasks_team_created_id_idx
    ON tasks (team_id, created_at DESC, id DESC);

INSERT INTO team_members (team_id, user_public_id, role)
SELECT team.id, app_user.user_id, 'viewer'
FROM teams AS team
JOIN public.orgs AS organization
  ON organization.schema_name = current_schema()
JOIN public.org_user AS organization_member
  ON organization_member.org_id = organization.id
JOIN public.users AS app_user
  ON app_user.id = organization_member.user_id
WHERE NOT team.is_quarantine
ON CONFLICT (team_id, user_public_id) DO NOTHING;
