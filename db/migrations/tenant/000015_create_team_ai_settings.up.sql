CREATE TABLE team_ai_settings (
    team_id BIGINT PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,
    endpoint_url TEXT NOT NULL,
    model TEXT NOT NULL,
    api_token TEXT NOT NULL
);
ALTER TABLE team_ai_settings OWNER TO synodus_owner;
ALTER TABLE team_ai_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_ai_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY team_ai_settings_owner ON team_ai_settings
    FOR ALL TO synodus_owner USING (true) WITH CHECK (true);
CREATE POLICY team_ai_settings_read ON team_ai_settings
    FOR SELECT TO synodus_runtime USING (
        team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
        AND synodus_has_team_membership(team_id)
    );
CREATE POLICY team_ai_settings_write ON team_ai_settings
    FOR ALL TO synodus_runtime USING (
        team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
        AND EXISTS (SELECT 1 FROM team_members m WHERE m.team_id = team_ai_settings.team_id
            AND m.user_public_id = synodus_app_user_public_id() AND m.role = 'team_admin')
    ) WITH CHECK (
        team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
        AND EXISTS (SELECT 1 FROM team_members m WHERE m.team_id = team_ai_settings.team_id
            AND m.user_public_id = synodus_app_user_public_id() AND m.role = 'team_admin')
    );
REVOKE ALL ON team_ai_settings FROM PUBLIC, synodus_runtime;
GRANT SELECT, INSERT, UPDATE ON team_ai_settings TO synodus_runtime;
