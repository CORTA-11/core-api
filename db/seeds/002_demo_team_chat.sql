-- Demo users for org 1 (University of Aratuwa). Password for all: password123
INSERT INTO users (id, org_id, active, email, password_hash, name, org_role)
VALUES
    (
        1,
        1,
        true,
        'admin@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=1,p=16$lGIJYC8KWzSYHsVrHOqRww$P2gIKJDS5lWODIIGuoldlx9IQzNhatmN4RTC93nyrhA',
        'Ada Admin',
        'ORG_ADMIN'
    ),
    (
        2,
        1,
        true,
        'leader@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=1,p=16$lGIJYC8KWzSYHsVrHOqRww$P2gIKJDS5lWODIIGuoldlx9IQzNhatmN4RTC93nyrhA',
        'Lee Leader',
        'ORG_USER'
    ),
    (
        3,
        1,
        true,
        'member@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=1,p=16$lGIJYC8KWzSYHsVrHOqRww$P2gIKJDS5lWODIIGuoldlx9IQzNhatmN4RTC93nyrhA',
        'Mo Member',
        'ORG_USER'
    )
ON CONFLICT (id) DO UPDATE
SET
    org_id = EXCLUDED.org_id,
    active = EXCLUDED.active,
    email = EXCLUDED.email,
    password_hash = EXCLUDED.password_hash,
    name = EXCLUDED.name,
    org_role = EXCLUDED.org_role;

SELECT setval(
    pg_get_serial_sequence('users', 'id'),
    (SELECT MAX(id) FROM users)
);

INSERT INTO organization_settings (org_id, allow_cloud_llm, retention_days)
VALUES (1, false, 30)
ON CONFLICT (org_id) DO NOTHING;

INSERT INTO teams (id, public_id, org_id, name, description, created_by_user)
VALUES (
    1,
    'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
    1,
    'Lab Alpha',
    'Demo research team for chat',
    1
)
ON CONFLICT (id) DO UPDATE
SET
    public_id = EXCLUDED.public_id,
    org_id = EXCLUDED.org_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    created_by_user = EXCLUDED.created_by_user,
    updated_at = now();

SELECT setval(
    pg_get_serial_sequence('teams', 'id'),
    (SELECT MAX(id) FROM teams)
);

-- Org admin (user 1) is intentionally not a team member — they only manage teams.
INSERT INTO team_members (team_id, user_id, role)
VALUES
    (1, 2, 'TEAM_LEADER'),
    (1, 3, 'CONTRIBUTOR')
ON CONFLICT (team_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

DELETE FROM team_members
WHERE team_id = 1
  AND user_id = 1;

INSERT INTO channels (id, team_id, name)
VALUES (
    '11111111-2222-3333-4444-555555555555',
    1,
    'general'
)
ON CONFLICT (id) DO UPDATE
SET
    team_id = EXCLUDED.team_id,
    name = EXCLUDED.name;
