package service

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type pgxPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type userService struct {
	pool            pgxPool
	queries         *repository.Queries
	tokenService    TokenService
	passwordService PasswordService
}

func NewUserService(pool pgxPool, queries *repository.Queries, tokenService TokenService, passwordService PasswordService) UserService {
	return &userService{
		pool:            pool,
		queries:         queries,
		tokenService:    tokenService,
		passwordService: passwordService,
	}
}

func (u *userService) GetUsers(ctx context.Context) ([]User, error) {
	repoUsers, err := u.queries.GetAllUsers(ctx)
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
	repoUser, err := u.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	domainUser := mapDBUserToDomain(repoUser)
	return &domainUser, nil
}

func (u *userService) CreateUser(ctx context.Context, name string, email string, password string) (*User, error) {
	hashedPassword, err := u.passwordService.HashPassword(password)
	if err != nil {
		return nil, err
	}

	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := u.queries.WithTx(tx)

	_, err = qtx.GetUserByEmail(ctx, email)
	if err == nil {
		return nil, ErrEmailAlreadyInUse
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	setPathPublicQuery := "SET LOCAL search_path TO public"
	_, err = tx.Exec(ctx, setPathPublicQuery)
	if err != nil {
		return nil, err
	}

	userParams := repository.CreateUserParams{
		Email:        email,
		PasswordHash: hashedPassword,
		DisplayName:  name,
	}

	repoUser, err := qtx.CreateUser(ctx, userParams)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
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

	hashedPassword, err := u.passwordService.HashPassword(password)
	if err != nil {
		return nil, err
	}

	params := repository.UpdateUserParams{
		UserID:       parsedUUID,
		Email:        email,
		PasswordHash: hashedPassword,
		DisplayName:  name,
	}

	repoUser, err := u.queries.UpdateUser(ctx, params)
	if err != nil {
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
	repoUser, err := u.queries.GetUserByEmail(ctx, email)
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
