// Package tenancy reconciles the public organization registry with isolated
// tenant schemas.
//
// The public registry records durable provisioning intent and lifecycle state;
// each tenant schema records applied migration versions and checksums in its own
// ledger; and the embedded MigrationSet is the binary's desired state.
// Reconciliation marks a tenant active only after these sources agree, and the
// API admits traffic only while the active registry record exactly matches its
// own embedded migration set.
//
// Reconciliation is resumable and safe to run concurrently. A per-organization
// advisory lock serializes schema changes, while each migration, ledger entry,
// and registry version update commits atomically. Schema names are derived only
// from server-owned organization UUIDs and never accepted from operators or
// clients.
package tenancy
