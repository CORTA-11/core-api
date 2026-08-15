package handlers

import (
	"context"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/stretchr/testify/mock"
)

type mockOrgStore struct {
	mock.Mock
}

func (m *mockOrgStore) GetOrgs(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockOrgStore) CreateOrg(ctx context.Context, arg repository.CreateOrgParams) (repository.Org, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(repository.Org), args.Error(1)
}
