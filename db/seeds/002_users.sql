INSERT INTO public.users (user_id, email, password_hash, display_name)
VALUES
    (
        '0d5a4f4e-8d3b-4f17-9a79-4c38e29a6d11',
        'admin@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=3,p=4$c3lub2R1cy1kZXYtc2VlZA$IcVOCmfhkOgf/e0KX7fXEv6s0LBKfSKvSUPEDZNuS9I',
        'Demo Administrator'
    ),
    (
        '48b38b47-36a8-4758-9858-c28c222d2c2e',
        'leader@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=3,p=4$c3lub2R1cy1kZXYtc2VlZA$IcVOCmfhkOgf/e0KX7fXEv6s0LBKfSKvSUPEDZNuS9I',
        'Demo Research Lead'
    ),
    (
        '981a7340-2a25-4aac-8b49-fddf45ff4894',
        'member@aratuwa.edu',
        '$argon2id$v=19$m=65536,t=3,p=4$c3lub2R1cy1kZXYtc2VlZA$IcVOCmfhkOgf/e0KX7fXEv6s0LBKfSKvSUPEDZNuS9I',
        'Demo Member'
    )
ON CONFLICT (email_canonical) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    display_name = EXCLUDED.display_name,
    deleted_at = NULL,
    updated_at = now();
