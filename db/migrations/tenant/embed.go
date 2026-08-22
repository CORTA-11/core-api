// Package tenantmigrations exposes the tenant migration source embedded in all
// API and provisioner binaries.
package tenantmigrations

import "embed"

// Files contains the ordered tenant up migrations.
//
//go:embed *.up.sql
var Files embed.FS
