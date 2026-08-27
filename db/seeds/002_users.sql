INSERT INTO public.users (user_id, email, password_hash, display_name, password_normalization)
VALUES
    (
        '0d5a4f4e-8d3b-4f17-9a79-4c38e29a6d11',
        'admin@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=3,p=4$4c1eXWZul7ikCrG2WIcAGA$a0EpoH1O/FnzybW8bTmTnrKJtkZMQuorWi2vfEWSUcs',
        'Demo Administrator',
        'nfc_v1'
    ),
    (
        '48b38b47-36a8-4758-9858-c28c222d2c2e',
        'leader@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=3,p=4$KnU2X0dZDpHcne6T7ZNNxg$GIaPvUCLidaJEb9B95K1i4R+WEckRsreDFjBhX4AwOE',
        'Demo Research Lead',
        'nfc_v1'
    ),
    (
        '981a7340-2a25-4aac-8b49-fddf45ff4894',
        'member@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=3,p=4$ZpBw2CZYs+ytbJPD9y33DQ$xzwBg3yWM+X9LNr41T+1Tvl6y+PdxRUThylv5RfIr2s',
        'Demo Member',
        'nfc_v1'
    )
ON CONFLICT (email_canonical) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    display_name = EXCLUDED.display_name,
    password_normalization = EXCLUDED.password_normalization,
    deleted_at = NULL,
    updated_at = now();
