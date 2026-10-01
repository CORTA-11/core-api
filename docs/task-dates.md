# Task date persistence

Task POST, PATCH and list responses support nullable `start_date` and `due_date` RFC 3339 timestamps. The frontend uses these to represent calendar days and generate Google Calendar events.

- POST stores supplied dates; omitted dates are null.
- PATCH preserves omitted dates, stores supplied timestamps, and clears dates explicitly set to null.
- Responses serialize dates in UTC, independent of the API process timezone.
- Setting `assignee_id` to null does not skip other fields in the same update.
- Team authorization and row-level security remain unchanged.

## Deployment

Tenant migration `000018_add_task_dates` adds nullable timestamp columns, leaving existing tasks undated. Follow the tenant migration rollout in `README.md`: stop the old provisioner, run the updated provisioner with provisioning credentials, reconcile tenants, and require `status --all` to report every non-deleting organization current before deploying the updated API. Then deploy the frontend date adapter.

Dates discarded by the previous API/adapter cannot be recovered; users must enter and save them again. No Google OAuth configuration is needed.

## Regression coverage

`TestTaskDatesPersistAcrossWritesAndReload` exercises real database creation, list reload, date-preserving moves, simultaneous unassignment/edits, clearing dates, and cross-tenant write rejection. Run it with the isolation build tag against an isolated test database. Task HTTP decoder tests cover date values, omission, explicit null and invalid input.
