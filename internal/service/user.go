package service

import (
	"context"
	"errors"
	"time"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = identity.ErrInvalidCredentials
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyInUse  = errors.New("email already in use")
)

type User struct {
	PublicID  uuid.UUID `json:"public_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	DeletedAt time.Time `json:"deleted_at"`
}

func mapDBUserToDomain(row publicdb.User) User {
	var deletedAt time.Time

	if row.DeletedAt.Valid {
		deletedAt = row.DeletedAt.Time
	}

	return User{
		PublicID:  row.UserID,
		Name:      row.DisplayName,
		Email:     row.Email,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		DeletedAt: deletedAt,
	}
}

type UserService interface {
	GetUsers(ctx context.Context) ([]User, error)
	GetUserByID(ctx context.Context, publicID string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	CreateUser(ctx context.Context, name string, email string, password string) (*User, error)
	UpdateUser(ctx context.Context, publicID string, name string, email string, password string) (*User, error)
	SoftDeleteUser(ctx context.Context, publicID string) (*User, error)
	Login(ctx context.Context, email string, password string) (string, *User, error)
}
