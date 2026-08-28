-- PostgreSQL checks UPDATE privilege for SELECT ... FOR UPDATE even though the
-- invitation application never exposes a general update operation.
GRANT UPDATE ON public.organization_invitations TO synodus_runtime;
