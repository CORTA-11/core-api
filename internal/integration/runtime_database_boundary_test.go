//go:build isolation

package integration_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeDatabaseBoundary(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()

	var sessionUser, currentUser string
	require.NoError(t, fixture.runtimePool.QueryRow(ctx, `SELECT session_user, current_user`).Scan(&sessionUser, &currentUser))
	assert.Equal(t, "synodus_runtime", sessionUser)
	assert.Equal(t, "synodus_runtime", currentUser)
	assert.EqualValues(t, 2, fixture.runtimePool.Config().MaxConns)

	expectedRoleAttributes := []string{
		"synodus_migrator:t:f:f:f:f:f:f",
		"synodus_owner:f:f:f:f:f:f:f",
		"synodus_provisioner:t:f:f:f:f:f:f",
		"synodus_runtime:t:f:f:f:f:f:f",
	}
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', rolname, rolcanlogin, rolsuper, rolcreaterole,
			rolcreatedb, rolinherit, rolreplication, rolbypassrls)
		FROM pg_roles
		WHERE rolname = ANY($1)
		ORDER BY rolname`, []any{[]string{"synodus_owner", "synodus_migrator", "synodus_provisioner", "synodus_runtime"}}, expectedRoleAttributes)

	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', member_role.rolname, membership.inherit_option, membership.set_option)
		FROM pg_auth_members AS membership
		JOIN pg_roles AS owner_role ON owner_role.oid = membership.roleid
		JOIN pg_roles AS member_role ON member_role.oid = membership.member
		WHERE owner_role.rolname = 'synodus_owner'
		ORDER BY member_role.rolname`, nil, []string{
		"synodus_migrator:f:t",
		"synodus_provisioner:f:t",
	})

	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', nspname, pg_get_userbyid(nspowner))
		FROM pg_namespace
		WHERE nspname = ANY($1)
		ORDER BY nspname`, []any{fixture.schemaNamesWithPublic()}, []string{
		fixture.orgs[0].schema + ":synodus_owner",
		fixture.orgs[1].schema + ":synodus_owner",
		"public:synodus_owner",
	})
	for _, schema := range fixture.schemaNamesWithPublic() {
		fixture.assertSchemaPrivileges(t, schema)
	}

	fixture.assertPublicCatalog(t)
	for _, organization := range fixture.orgs {
		fixture.assertTenantCatalog(t, organization)
	}
	fixture.assertRuntimeDenials(t)
}

func (fixture *tenantBoundaryFixture) assertPublicCatalog(t *testing.T) {
	t.Helper()
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', tablename, tableowner)
		FROM pg_tables
		WHERE schemaname = 'public' AND tablename = ANY($1)
		ORDER BY tablename`, []any{[]string{"orgs", "users", "org_user", "organization_invitations", "schema_migrations", "sessions", "user_public_keys"}}, []string{
		"org_user:synodus_owner",
		"organization_invitations:synodus_owner",
		"orgs:synodus_owner",
		"schema_migrations:synodus_owner",
		"sessions:synodus_owner",
		"user_public_keys:synodus_owner",
		"users:synodus_owner",
	})
	assertCatalogRows(t, fixture, runtimeTableGrantsSQL, []any{[]string{"public"}}, []string{
		"public:org_user:DELETE", "public:org_user:INSERT", "public:org_user:SELECT", "public:org_user:UPDATE",
		"public:organization_invitations:DELETE", "public:organization_invitations:INSERT",
		"public:organization_invitations:SELECT", "public:organization_invitations:UPDATE",
		"public:orgs:DELETE", "public:orgs:INSERT", "public:orgs:SELECT", "public:orgs:UPDATE",
		"public:sessions:DELETE", "public:sessions:INSERT", "public:sessions:SELECT", "public:sessions:UPDATE",
		"public:user_public_keys:DELETE", "public:user_public_keys:INSERT", "public:user_public_keys:SELECT", "public:user_public_keys:UPDATE",
		"public:users:DELETE", "public:users:INSERT", "public:users:SELECT", "public:users:UPDATE",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', sequencename, sequenceowner)
		FROM pg_sequences
		WHERE schemaname = 'public' AND sequencename = ANY($1)
		ORDER BY sequencename`, []any{[]string{"organization_invitations_id_seq", "orgs_id_seq", "sessions_id_seq", "users_id_seq"}}, []string{
		"organization_invitations_id_seq:synodus_owner", "orgs_id_seq:synodus_owner", "sessions_id_seq:synodus_owner", "users_id_seq:synodus_owner",
	})
	for _, sequence := range []string{"organization_invitations_id_seq", "orgs_id_seq", "sessions_id_seq", "users_id_seq"} {
		fixture.assertSequencePrivileges(t, "public", sequence, true, true, false)
	}
}

func (fixture *tenantBoundaryFixture) assertTenantCatalog(t *testing.T, organization tenantBoundaryOrganization) {
	t.Helper()
	schema := organization.schema
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', tablename, tableowner)
		FROM pg_tables
		WHERE schemaname = $1
		ORDER BY tablename`, []any{schema}, []string{
		"files:synodus_owner",
		"resource_requests:synodus_owner",
		"resources:synodus_owner",
		"schema_migrations:synodus_owner",
		"tasks:synodus_owner",
		"team_members:synodus_owner",
		"team_shared_keys:synodus_owner",
		"teams:synodus_owner",
	})
	assertCatalogRows(t, fixture, runtimeTableGrantsSQL, []any{[]string{schema}}, []string{
		schema + ":files:DELETE", schema + ":files:INSERT", schema + ":files:SELECT", schema + ":files:UPDATE",
		schema + ":resource_requests:DELETE", schema + ":resource_requests:INSERT", schema + ":resource_requests:SELECT", schema + ":resource_requests:UPDATE",
		schema + ":resources:DELETE", schema + ":resources:INSERT", schema + ":resources:SELECT", schema + ":resources:UPDATE",
		schema + ":tasks:DELETE", schema + ":tasks:INSERT", schema + ":tasks:SELECT", schema + ":tasks:UPDATE",
		schema + ":team_members:SELECT",
		schema + ":team_shared_keys:DELETE", schema + ":team_shared_keys:INSERT", schema + ":team_shared_keys:SELECT", schema + ":team_shared_keys:UPDATE",
		schema + ":teams:SELECT",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', relation.relname, relation.relrowsecurity, relation.relforcerowsecurity)
		FROM pg_class AS relation
		JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = $1 AND relation.relname = ANY($2)
		ORDER BY relation.relname`, []any{schema, []string{"teams", "team_members", "tasks", "resources", "resource_requests"}}, []string{
		"resource_requests:t:t", "resources:t:t", "tasks:t:t", "team_members:t:t", "teams:t:t",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', tablename, policyname, cmd, array_to_string(roles, ','))
		FROM pg_policies
		WHERE schemaname = $1
		ORDER BY tablename, policyname`, []any{schema}, []string{
		"files:files_owner_maintenance:ALL:synodus_owner",
		"files:files_runtime_access:ALL:synodus_runtime",
		"resource_requests:resource_requests_owner_maintenance:ALL:synodus_owner",
		"resource_requests:resource_requests_runtime_access:ALL:synodus_runtime",
		"resources:resources_owner_maintenance:ALL:synodus_owner",
		"resources:resources_runtime_access:ALL:synodus_runtime",
		"tasks:tasks_owner_maintenance:ALL:synodus_owner",
		"tasks:tasks_runtime_access:ALL:synodus_runtime",
		"team_members:team_members_owner_maintenance:ALL:synodus_owner",
		"team_members:team_members_runtime_select:SELECT:synodus_runtime",
		"team_shared_keys:team_shared_keys_owner_maintenance:ALL:synodus_owner",
		"team_shared_keys:team_shared_keys_runtime_access:ALL:synodus_runtime",
		"teams:teams_owner_maintenance:ALL:synodus_owner",
		"teams:teams_runtime_organization_select:SELECT:synodus_runtime",
		"teams:teams_runtime_select:SELECT:synodus_runtime",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', relation.relname, constraint_name.conname, constraint_name.contype)
		FROM pg_constraint AS constraint_name
		JOIN pg_class AS relation ON relation.oid = constraint_name.conrelid
		JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = $1 AND relation.relname = ANY($2)
		ORDER BY relation.relname, constraint_name.conname`, []any{schema, []string{"teams", "team_members", "tasks"}}, []string{
		"tasks:tasks_assignee_public_id_fk:f", "tasks:tasks_created_at_not_null:n", "tasks:tasks_description_not_null:n", "tasks:tasks_id_not_null:n",
		"tasks:tasks_pk:p", "tasks:tasks_public_id_not_null:n", "tasks:tasks_public_id_unique:u",
		"tasks:tasks_status_check:c", "tasks:tasks_status_not_null:n", "tasks:tasks_team_fk:f",
		"tasks:tasks_team_id_not_null:n", "tasks:tasks_updated_at_not_null:n",
		"team_members:team_members_created_at_not_null:n", "team_members:team_members_pk:p",
		"team_members:team_members_role_check:c", "team_members:team_members_role_not_null:n",
		"team_members:team_members_team_fk:f", "team_members:team_members_team_id_not_null:n",
		"team_members:team_members_updated_at_not_null:n", "team_members:team_members_user_public_fk:f",
		"team_members:team_members_user_public_id_not_null:n",
		"teams:teams_created_at_not_null:n", "teams:teams_id_not_null:n", "teams:teams_is_quarantine_not_null:n",
		"teams:teams_name_not_null:n", "teams:teams_name_unique:u", "teams:teams_pk:p",
		"teams:teams_public_id_not_null:n", "teams:teams_public_id_unique:u", "teams:teams_slug_not_null:n",
		"teams:teams_slug_unique:u", "teams:teams_updated_at_not_null:n",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', tablename, indexname)
		FROM pg_indexes
		WHERE schemaname = $1 AND tablename = ANY($2)
		ORDER BY tablename, indexname`, []any{schema, []string{"teams", "team_members", "tasks"}}, []string{
		"tasks:tasks_assignee_team_idx", "tasks:tasks_pk", "tasks:tasks_public_id_unique", "tasks:tasks_team_created_id_idx",
		"team_members:team_members_pk", "team_members:team_members_user_team_idx",
		"teams:teams_name_unique", "teams:teams_pk", "teams:teams_public_id_unique", "teams:teams_single_quarantine_idx", "teams:teams_slug_unique",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', routine_name, privilege_type)
		FROM information_schema.routine_privileges
		WHERE specific_schema = $1 AND grantee = 'synodus_runtime'
		ORDER BY routine_name`, []any{schema}, []string{
		"add_team_contributor:EXECUTE",
		"create_team_with_creator:EXECUTE",
		"create_team_with_creator:EXECUTE",
		"list_bound_team_members:EXECUTE",
		"synodus_app_user_public_id:EXECUTE",
		"synodus_current_organization_role:EXECUTE",
		"synodus_has_organization_membership:EXECUTE",
		"synodus_has_team_membership:EXECUTE",
		"synodus_user_display_name:EXECUTE",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', procedure.proname, pg_get_userbyid(procedure.proowner))
		FROM pg_proc AS procedure
		JOIN pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
		WHERE namespace.nspname = $1 AND procedure.proname = ANY($2)
		ORDER BY procedure.proname`, []any{schema, []string{
		"add_team_contributor", "create_team_with_creator", "list_bound_team_members", "synodus_app_user_public_id", "synodus_has_team_membership",
	}}, []string{
		"add_team_contributor:synodus_owner",
		"create_team_with_creator:synodus_owner",
		"create_team_with_creator:synodus_owner",
		"list_bound_team_members:synodus_owner",
		"synodus_app_user_public_id:synodus_owner",
		"synodus_has_team_membership:synodus_owner",
	})
	assertCatalogRows(t, fixture, `
		SELECT concat_ws(':', sequencename, sequenceowner)
		FROM pg_sequences
		WHERE schemaname = $1 AND sequencename = ANY($2)
		ORDER BY sequencename`, []any{schema, []string{"resource_requests_id_seq", "resources_id_seq", "tasks_id_seq", "teams_id_seq"}}, []string{
		"resource_requests_id_seq:synodus_owner", "resources_id_seq:synodus_owner", "tasks_id_seq:synodus_owner", "teams_id_seq:synodus_owner",
	})
	fixture.assertSequencePrivileges(t, schema, "resource_requests_id_seq", true, true, false)
	fixture.assertSequencePrivileges(t, schema, "resources_id_seq", true, true, false)
	fixture.assertSequencePrivileges(t, schema, "tasks_id_seq", true, true, false)
	fixture.assertSequencePrivileges(t, schema, "teams_id_seq", false, false, false)

	var teamIDNullable string
	require.NoError(t, fixture.adminPool.QueryRow(context.Background(), `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'tasks' AND column_name = 'team_id'`, schema).Scan(&teamIDNullable))
	assert.Equal(t, "NO", teamIDNullable)
}

func (fixture *tenantBoundaryFixture) assertSchemaPrivileges(t *testing.T, schema string) {
	t.Helper()
	var usage, createPrivilege bool
	require.NoError(t, fixture.adminPool.QueryRow(context.Background(), `
		SELECT has_schema_privilege('synodus_runtime', $1, 'USAGE'),
		       has_schema_privilege('synodus_runtime', $1, 'CREATE')`, schema).Scan(&usage, &createPrivilege))
	assert.True(t, usage, schema)
	assert.False(t, createPrivilege, schema)
}

func (fixture *tenantBoundaryFixture) assertSequencePrivileges(
	t *testing.T,
	schema string,
	sequence string,
	wantUsage bool,
	wantSelect bool,
	wantUpdate bool,
) {
	t.Helper()
	qualified := pgx.Identifier{schema, sequence}.Sanitize()
	var usage, selectPrivilege, update bool
	require.NoError(t, fixture.adminPool.QueryRow(context.Background(), `
		SELECT has_sequence_privilege('synodus_runtime', $1, 'USAGE'),
		       has_sequence_privilege('synodus_runtime', $1, 'SELECT'),
		       has_sequence_privilege('synodus_runtime', $1, 'UPDATE')`, qualified).Scan(&usage, &selectPrivilege, &update))
	assert.Equal(t, wantUsage, usage, qualified)
	assert.Equal(t, wantSelect, selectPrivilege, qualified)
	assert.Equal(t, wantUpdate, update, qualified)
}

func (fixture *tenantBoundaryFixture) assertRuntimeDenials(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := fixture.runtimePool.Exec(ctx, `SELECT version FROM public.schema_migrations`)
	assert.Error(t, err)
	for _, organization := range fixture.orgs {
		ledger := pgx.Identifier{organization.schema, "schema_migrations"}.Sanitize()
		_, err = fixture.runtimePool.Exec(ctx, `SELECT version FROM `+ledger)
		assert.Error(t, err, organization.schema)
	}
	_, err = fixture.runtimePool.Exec(ctx, `CREATE SCHEMA tenant_boundary_runtime_must_not_create`)
	assert.Error(t, err)
	_, err = fixture.runtimePool.Exec(ctx, `SET ROLE synodus_owner`)
	assert.Error(t, err)
	_, err = fixture.runtimePool.Exec(ctx, `ALTER TABLE `+pgx.Identifier{fixture.orgs[0].schema, "tasks"}.Sanitize()+` DISABLE ROW LEVEL SECURITY`)
	assert.Error(t, err)
}

const runtimeTableGrantsSQL = `
	SELECT concat_ws(':', table_schema, table_name, privilege_type)
	FROM information_schema.role_table_grants
	WHERE grantee = 'synodus_runtime' AND table_schema = ANY($1)
	ORDER BY table_schema, table_name, privilege_type`

func (fixture *tenantBoundaryFixture) schemaNamesWithPublic() []string {
	return []string{"public", fixture.orgs[0].schema, fixture.orgs[1].schema}
}

func assertCatalogRows(t *testing.T, fixture *tenantBoundaryFixture, query string, arguments []any, want []string) {
	t.Helper()
	rows, err := fixture.adminPool.Query(context.Background(), query, arguments...)
	require.NoError(t, err)
	defer rows.Close()
	got := make([]string, 0, len(want))
	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))
		got = append(got, value)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, want, got, fmt.Sprintf("catalog query: %s", query))
}
