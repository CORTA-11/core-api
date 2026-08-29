package tenantdb

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const listBoundTeamMembers = `
SELECT user_public_id, display_name, email, role, joined_at
FROM list_bound_team_members()
ORDER BY joined_at, user_public_id
LIMIT $1`

type BoundTeamMember struct {
	UserPublicID uuid.UUID
	DisplayName  string
	Email        string
	Role         string
	JoinedAt     time.Time
}

// ListBoundTeamMembers uses the tenant's security-definer function so identity
// data is returned only for members of the team bound to the transaction.
func (q *Queries) ListBoundTeamMembers(ctx context.Context, limit int32) ([]BoundTeamMember, error) {
	rows, err := q.db.Query(ctx, listBoundTeamMembers, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]BoundTeamMember, 0)
	for rows.Next() {
		var item BoundTeamMember
		if err := rows.Scan(&item.UserPublicID, &item.DisplayName, &item.Email, &item.Role, &item.JoinedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
