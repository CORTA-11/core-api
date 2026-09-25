-- name: GetTeamAISettings :one
SELECT endpoint_url, model, api_token FROM team_ai_settings
WHERE team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT;

-- name: SaveTeamAISettings :one
INSERT INTO team_ai_settings (team_id, endpoint_url, model, api_token)
VALUES (NULLIF(current_setting('app.team_id', true), '')::BIGINT,
    sqlc.arg('endpoint_url'), sqlc.arg('model'), sqlc.arg('api_token'))
ON CONFLICT (team_id) DO UPDATE SET endpoint_url = EXCLUDED.endpoint_url,
    model = EXCLUDED.model,
    api_token = CASE WHEN EXCLUDED.api_token = '' THEN team_ai_settings.api_token ELSE EXCLUDED.api_token END
RETURNING endpoint_url, model, api_token;

-- name: ListAIChatMessages :many
SELECT public_id, sender_user_public_id, message, created_at FROM chat_messages
WHERE team_id = NULLIF(current_setting('app.team_id', true), '')::BIGINT
    AND deleted_at IS NULL AND btrim(message) <> ''
    AND created_at >= sqlc.arg('from_time')::timestamptz
    AND created_at <= sqlc.arg('to_time')::timestamptz
ORDER BY created_at, public_id
LIMIT sqlc.arg('limit');
