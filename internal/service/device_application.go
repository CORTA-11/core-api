package service

import (
	"context"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
)

type DeviceApplication struct {
	queries *publicdb.Queries
}

func NewDeviceApplication(queries *publicdb.Queries) *DeviceApplication {
	return &DeviceApplication{queries: queries}
}

func (app *DeviceApplication) RegisterDevice(ctx context.Context, userID uuid.UUID, token string, platform string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidInput
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = "android"
	}
	_, err := app.queries.UpsertDeviceToken(ctx, publicdb.UpsertDeviceTokenParams{
		UserID:   userID,
		Token:    token,
		Platform: platform,
	})
	return err
}

func (app *DeviceApplication) DeleteDevice(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidInput
	}
	return app.queries.DeleteDeviceToken(ctx, token)
}
