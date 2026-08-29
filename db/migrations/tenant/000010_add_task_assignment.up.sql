-- An assignee must be a member of the owning team; membership is proven by the
-- security-definer synodus_has_team_membership() and the tasks RLS policy.
ALTER TABLE tasks
    ADD COLUMN assignee_public_id UUID
        CONSTRAINT tasks_assignee_public_id_fk
        REFERENCES public.users(user_id)
        ON DELETE SET NULL;

CREATE INDEX tasks_assignee_team_idx
    ON tasks (assignee_public_id, team_id)
    WHERE assignee_public_id IS NOT NULL;