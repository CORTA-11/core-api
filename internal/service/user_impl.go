package service

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type pgxPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type userService struct {
	pool            pgxPool
	queries         *publicdb.Queries
	tokenService    TokenService
	passwordService PasswordService
}

func NewUserService(pool pgxPool, queries *publicdb.Queries, tokenService TokenService, passwordService PasswordService) UserService {
	return &userService{
		pool:            pool,
		queries:         queries,
		tokenService:    tokenService,
		passwordService: passwordService,
	}
}

func (u *userService) GetUsers(ctx context.Context) ([]User, error) {
	repoUsers, err := u.queries.GetAllUsers(ctx, listResultLimit)
	if err != nil {
		return nil, err
	}

	users := make([]User, len(repoUsers))
	for i, repoUser := range repoUsers {
		users[i] = mapDBUserToDomain(repoUser)
	}

	return users, nil
}

func (u *userService) GetUserByID(ctx context.Context, publicID string) (*User, error) {
	parsedUUID, err := uuid.Parse(publicID)
	if err != nil {
		return nil, err
	}

	repoUser, err := u.queries.GetUserByID(ctx, parsedUUID)
	if err != nil {
		return nil, err
	}

	domainUser := mapDBUserToDomain(repoUser)
	return &domainUser, nil
}

func (u *userService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	canonical, err := (identity.EmailCanonicalizer{}).Canonicalize(email)
	if err != nil {
		return nil, err
	}
	repoUser, err := u.queries.GetUserByCanonicalEmail(ctx, canonical.Key)
	if err != nil {
		return nil, err
	}

	domainUser := mapDBUserToDomain(repoUser)
	return &domainUser, nil
}

func (u *userService) CreateUser(ctx context.Context, name string, email string, password string) (*User, error) {
	canonical, err := (identity.EmailCanonicalizer{}).Canonicalize(email)
	if err != nil {
		return nil, err
	}
	hashedPassword, err := u.passwordService.HashPassword(password)
	if err != nil {
		return nil, err
	}

	userParams := publicdb.CreateUserParams{
		Email:        canonical.Display,
		PasswordHash: hashedPassword,
		DisplayName:  name,
	}

	repoUser, err := u.queries.CreateUser(ctx, userParams)
	if err != nil {
		if isCanonicalEmailConflict(err) {
			return nil, ErrEmailAlreadyInUse
		}
		return nil, err
	}

	domainUser := mapDBUserToDomain(repoUser)
	return &domainUser, nil
}

func (u *userService) UpdateUser(ctx context.Context, publicID string, name string, email string, password string) (*User, error) {
	parsedUUID, err := uuid.Parse(publicID)
	if err != nil {
		return nil, err
	}

	canonical, err := (identity.EmailCanonicalizer{}).Canonicalize(email)
	if err != nil {
		return nil, err
	}

	hashedPassword, err := u.passwordService.HashPassword(password)
	if err != nil {
		return nil, err
	}

	params := publicdb.UpdateUserParams{
		UserID:       parsedUUID,
		Email:        canonical.Display,
		PasswordHash: hashedPassword,
		DisplayName:  name,
	}

	repoUser, err := u.queries.UpdateUser(ctx, params)
	if err != nil {
		if isCanonicalEmailConflict(err) {
			return nil, ErrEmailAlreadyInUse
		}
		return nil, err
	}

	domainUser := mapDBUserToDomain(repoUser)
	return &domainUser, nil
}

func (u *userService) SoftDeleteUser(ctx context.Context, publicID string) (*User, error) {
	parsedUUID, err := uuid.Parse(publicID)
	if err != nil {
		return nil, err
	}

	repoUser, err := u.queries.SoftDeleteUser(ctx, parsedUUID)
	if err != nil {
		return nil, err
	}

	domainUser := mapDBUserToDomain(repoUser)
	return &domainUser, nil
}

func (u *userService) Login(ctx context.Context, email string, password string) (string, *User, error) {
	canonical, err := (identity.EmailCanonicalizer{}).Canonicalize(email)
	if err != nil {
		return "", nil, ErrInvalidCredentials
	}
	repoUser, err := u.queries.GetUserByCanonicalEmail(ctx, canonical.Key)
	if err != nil {
		return "", nil, ErrInvalidCredentials
	}

	match, err := u.passwordService.VerifyPassword(password, repoUser.PasswordHash)
	if err != nil || !match {
		return "", nil, ErrInvalidCredentials
	}

	token, err := u.tokenService.GenerateToken(repoUser.UserID, repoUser.Email)
	if err != nil {
		return "", nil, err
	}

	domainUser := mapDBUserToDomain(repoUser)
	return token, &domainUser, nil
}

func isCanonicalEmailConflict(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505" &&
		postgresError.ConstraintName == "users_email_canonical_unique"
}
