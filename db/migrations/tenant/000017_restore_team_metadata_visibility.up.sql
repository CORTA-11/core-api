-- Shared-resource bookings join team metadata to return occupied time slots,
-- with other teams' identifying details redacted by the booking query.
-- Restrict discovery in GetTeamsAfter/GetTeamsBefore, not this shared policy.
ALTER POLICY teams_runtime_organization_select ON teams
    USING (NOT is_quarantine AND synodus_has_organization_membership());
