ALTER POLICY teams_runtime_organization_select ON teams
    USING (
        NOT is_quarantine
        AND synodus_has_organization_membership()
        AND synodus_current_organization_role() IN ('owner', 'administrator')
    );
