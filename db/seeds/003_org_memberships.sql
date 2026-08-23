WITH desired_memberships (email, organization_public_id) AS (
    VALUES
        ('admin@aratuwa.edu', '30ee7153-9b48-4560-8cbf-972587a60fda'::uuid),
        ('admin@aratuwa.edu', 'f1810095-f8a0-4e27-83df-d88b3256604d'::uuid),
        ('admin@aratuwa.edu', 'afb118ba-2ade-4422-9f20-04754fd1d4a7'::uuid),
        ('leader@aratuwa.edu', '30ee7153-9b48-4560-8cbf-972587a60fda'::uuid),
        ('leader@aratuwa.edu', 'f1810095-f8a0-4e27-83df-d88b3256604d'::uuid),
        ('member@aratuwa.edu', '30ee7153-9b48-4560-8cbf-972587a60fda'::uuid)
)
INSERT INTO public.org_user (org_id, user_id)
SELECT organization.id, app_user.id
FROM desired_memberships AS desired
JOIN public.orgs AS organization
  ON organization.public_id = desired.organization_public_id
JOIN public.users AS app_user
  ON app_user.email = desired.email
WHERE NOT EXISTS (
    SELECT 1
    FROM public.org_user AS existing
    WHERE existing.org_id = organization.id
      AND existing.user_id = app_user.id
);
