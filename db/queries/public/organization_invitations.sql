-- name: CreateOrganizationInvitation :one
INSERT INTO public.organization_invitations (org_id, email_hmac, token_hash, expires_at)
VALUES (sqlc.arg('org_id'), sqlc.arg('email_hmac'), sqlc.arg('token_hash'), now() + interval '7 days')
RETURNING id, public_id, org_id, email_hmac, token_hash, created_at, expires_at;

-- name: LockOrganizationInvitationAdminContext :one
SELECT organization.id AS org_id, membership.role, organization.lifecycle_state,
       (SELECT count(*) FROM public.org_user AS owners
        WHERE owners.org_id = organization.id AND owners.role = 'owner') AS owner_count
FROM public.orgs AS organization
JOIN public.org_user AS membership ON membership.org_id = organization.id
JOIN public.users AS actor ON actor.id = membership.user_id
WHERE organization.public_id = sqlc.arg('organization_public_id')
  AND organization.deleted_at IS NULL
  AND actor.user_id = sqlc.arg('user_public_id')
  AND actor.deleted_at IS NULL
FOR UPDATE OF organization, membership;

-- name: ListOrganizationInvitationsAfter :many
SELECT invitation.id, invitation.public_id, invitation.org_id, invitation.created_at, invitation.expires_at
FROM public.organization_invitations AS invitation
WHERE invitation.org_id = sqlc.arg('org_id')
  AND invitation.expires_at > now()
  AND (invitation.created_at, invitation.public_id) >
      (sqlc.arg('after_created_at'), sqlc.arg('after_public_id')::uuid)
ORDER BY invitation.created_at, invitation.public_id
LIMIT sqlc.arg('limit');

-- name: DeleteOrganizationInvitation :one
DELETE FROM public.organization_invitations
WHERE public_id = sqlc.arg('public_id') AND org_id = sqlc.arg('org_id')
RETURNING public_id;

-- name: GetOrganizationInvitationByTokenHash :one
SELECT invitation.id, invitation.public_id, invitation.org_id, invitation.email_hmac,
       invitation.created_at, invitation.expires_at, organization.public_id AS organization_public_id,
       organization.name AS organization_name
FROM public.organization_invitations AS invitation
JOIN public.orgs AS organization ON organization.id = invitation.org_id
WHERE invitation.token_hash = sqlc.arg('token_hash')
  AND invitation.expires_at > now()
  AND organization.deleted_at IS NULL
  AND organization.lifecycle_state = 'active'
FOR UPDATE OF invitation;

-- name: AcceptOrganizationInvitation :one
WITH consumed AS (
    DELETE FROM public.organization_invitations
    WHERE id = sqlc.arg('invitation_id')
    RETURNING org_id
), added AS (
    INSERT INTO public.org_user (org_id, user_id, role)
    SELECT consumed.org_id, sqlc.arg('user_id'), 'member'
    FROM consumed
    ON CONFLICT (org_id, user_id) DO NOTHING
    RETURNING org_id
)
SELECT consumed.org_id FROM consumed;

-- name: DeclineOrganizationInvitation :one
DELETE FROM public.organization_invitations
WHERE id = sqlc.arg('invitation_id')
RETURNING public_id;

-- name: PurgeExpiredOrganizationInvitations :execrows
WITH expired AS (
    SELECT id FROM public.organization_invitations
    WHERE expires_at <= now()
    ORDER BY expires_at
    LIMIT sqlc.arg('limit')
    FOR UPDATE SKIP LOCKED
)
DELETE FROM public.organization_invitations AS invitation
USING expired WHERE invitation.id = expired.id;

-- name: GetActiveUserInvitationIdentity :one
SELECT id, email_canonical
FROM public.users
WHERE user_id = sqlc.arg('user_public_id') AND deleted_at IS NULL;
