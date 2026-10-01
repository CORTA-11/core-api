# Creator-controlled document and file access

Team members can browse titles/file metadata and request permission. Only the
immutable creator can approve/deny requests or grant access directly to a current
member. Team and organization administrators do not inherit creator authority.
A grant covers a single resource, not every document or file in the team.

## Enforcement

Tenant `content_owners` rows retain the initial uploader/document creator. Insert
triggers register ownership; document deletion and file soft deletion remove the
owner and cascade its requests. Document edits never transfer ownership.

`content_access_requests` has one current row per resource/requester. Requests
transition `pending -> granted|denied`; only `denied` can return to `pending`.
Decisions conditionally update pending rows, so concurrent decisions cannot both
succeed. Direct creator grants can create or replace the current status.

RLS restricts ownership metadata to the bound team, and request visibility to
its requester and creator. The service enters tenant/team context through the
authorizer on every operation. Content queries additionally call
`synodus_can_access_content`: the creator or a granted requester can fetch file
bytes or read/update document projections and collaboration state. Document
lists select metadata only, not the HTML body. Socket tickets use the same read
check. Current team membership remains mandatory after a grant.

The HTTP operations and payloads are defined in `api/openapi.yaml`, under the
team-scoped `/content-access` paths. Mutation endpoints require session cookies
and CSRF. An unavailable resource, unauthorized creator action, or stale/repeated
transition returns 404 without disclosing protected information.

## Realtime and encryption

`GET .../content-access/events` emits the `content-access` SSE event with a full
privacy-filtered snapshot, initially and whenever it changes. Database polling
supports multiple API replicas and disconnected clients without an in-memory
notification bus. Membership is rechecked every two seconds; heartbeat comments
keep unchanged streams alive. Thirty-second connection lifetimes force session
revalidation on automatic browser reconnection.

The frontend shares an account/org/team-scoped React Query cache. One stream in
the current team header refreshes controls and notifications. Notification keys
include request ID, status, and update timestamp so a retried request's decision
is unread again. Creator notifications cover incoming pending requests;
requesters receive approval/denial notifications.

File permission and encryption-key possession are independent. Before approving
or directly granting a file through the UI, its creator's browser re-wraps only
that file's team-key version for the recipient if needed. Possessing that team
key does not authorize downloading other files encrypted with it. If preparation
fails (for example, the recipient has not set up keys), the permission mutation
is not submitted. Documents use the existing collaboration transport.

## Rollout

Apply tenant migration **000019** to every organization before deploying the new
API/frontend together, using the normal migration/provisioning workflow.
Files use their recorded uploader as creator. **Older documents never recorded
an immutable creator: the migration assigns their last editor as initial owner.**
Review this assignment against external audit records if exact historical
ownership matters. New documents always retain their original creator. Existing
team members need explicit grants for old content after this migration.

The down migration removes permissions and restores the previous team-wide
behavior when paired with the previous application version; it destroys request
and grant records. Keep backups before rolling back.
