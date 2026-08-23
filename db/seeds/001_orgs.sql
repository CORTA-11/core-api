INSERT INTO public.orgs (public_id, name, schema_name)
VALUES
    ('30ee7153-9b48-4560-8cbf-972587a60fda', 'University of Aratuwa', 'org_30ee71539b4845608cbf972587a60fda'),
    ('f1810095-f8a0-4e27-83df-d88b3256604d', 'MedSync', 'org_f1810095f8a04e2783dfd88b3256604d'),
    ('afb118ba-2ade-4422-9f20-04754fd1d4a7', 'Pied Piper', 'org_afb118ba2ade44229f2004754fd1d4a7')
ON CONFLICT (public_id) DO UPDATE
SET name = EXCLUDED.name;
