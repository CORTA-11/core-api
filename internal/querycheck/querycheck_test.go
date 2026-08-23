package querycheck

import (
	"strings"
	"testing"
)

func TestCheckRejectsUnsafeOrUnboundedQueries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		query     string
		wantIssue string
	}{
		{
			name:      "bare select wildcard",
			query:     "-- name: GetUsers :one\nSELECT * FROM public.users;",
			wantIssue: "wildcard projection",
		},
		{
			name:      "qualified select wildcard",
			query:     "-- name: GetUsers :one\nSELECT u.* FROM public.users AS u;",
			wantIssue: "wildcard projection",
		},
		{
			name:      "qualified wildcard after explicit column",
			query:     "-- name: GetUsers :one\nSELECT o.id, u.* FROM public.users AS u JOIN public.org_user AS o ON o.user_id = u.id;",
			wantIssue: "wildcard projection",
		},
		{
			name:      "wildcard returning",
			query:     "-- name: CreateUser :one\nINSERT INTO public.users (email) VALUES ($1) RETURNING *;",
			wantIssue: "wildcard RETURNING",
		},
		{
			name:      "qualified wildcard returning",
			query:     "-- name: CreateUser :one\nINSERT INTO public.users AS u (email) VALUES ($1) RETURNING u.*;",
			wantIssue: "wildcard RETURNING",
		},
		{
			name:      "many without limit",
			query:     "-- name: GetUsers :many\nSELECT id FROM public.users ORDER BY created_at, id;",
			wantIssue: "parameterized LIMIT",
		},
		{
			name:      "many with literal limit",
			query:     "-- name: GetUsers :many\nSELECT id FROM public.users ORDER BY created_at, id LIMIT 100;",
			wantIssue: "parameterized LIMIT",
		},
		{
			name:      "many without order",
			query:     "-- name: GetUsers :many\nSELECT id FROM public.users LIMIT sqlc.arg('limit');",
			wantIssue: "ORDER BY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			issues := Check("fixture.sql", []byte(tt.query))
			if len(issues) == 0 {
				t.Fatalf("Check() returned no issues, want issue containing %q", tt.wantIssue)
			}
			if !strings.Contains(issues[0].Error(), "fixture.sql") ||
				!strings.Contains(issues[0].Error(), tt.wantIssue) {
				t.Fatalf("Check() issue = %q, want file and %q", issues[0], tt.wantIssue)
			}
		})
	}
}

func TestCheckAcceptsExplicitBoundedQueries(t *testing.T) {
	t.Parallel()

	query := `-- name: CountUsers :one
SELECT COUNT(*) FROM public.users;

-- name: GetUsers :many
SELECT id, email
FROM public.users
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: CreateUser :one
INSERT INTO public.users (email)
VALUES ($1)
RETURNING id, email;
`

	if issues := Check("valid.sql", []byte(query)); len(issues) != 0 {
		t.Fatalf("Check() issues = %v, want none", issues)
	}
}

func TestCheckReportsQueryName(t *testing.T) {
	t.Parallel()

	query := "-- name: GetUsers :many\nSELECT id FROM public.users;"
	issues := Check("users.sql", []byte(query))
	if len(issues) == 0 || !strings.Contains(issues[0].Error(), "GetUsers") {
		t.Fatalf("Check() issues = %v, want query name GetUsers", issues)
	}
}

func TestCheckSchemaBoundaryRejectsProductionBypassAndAllowsOwnedPaths(t *testing.T) {
	t.Parallel()
	unsafe := []byte("func bypass() { tx.Exec(ctx, `SET LOCAL search_path TO org_forged`) }")
	issues := CheckSchemaBoundary("internal/service/bypass.go", unsafe)
	if len(issues) != 1 || !strings.Contains(issues[0].Error(), "outside the explicit allowlist") {
		t.Fatalf("CheckSchemaBoundary() issues = %v, want one allowlist issue", issues)
	}
	if issues := CheckSchemaBoundary("internal/tenancy/executor.go", unsafe); len(issues) != 0 {
		t.Fatalf("trusted executor issues = %v, want none", issues)
	}
	if issues := CheckSchemaBoundary("internal/testsupport/integration.go", []byte("CREATE SCHEMA fixture")); len(issues) != 0 {
		t.Fatalf("test fixture issues = %v, want none", issues)
	}
}
