package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrAISettingsMissing = errors.New("team admin must configure AI settings")

type TeamAISettingsInput struct {
	EndpointURL string `json:"endpoint_url"`
	Model       string `json:"model"`
	APIToken    string `json:"api_token"`
}
type TeamAISettingsView struct {
	EndpointURL string `json:"endpoint_url"`
	Model       string `json:"model"`
	HasAPIToken bool   `json:"has_api_token"`
}

func (a *AIApplication) GetSettings(ctx context.Context, p session.Principal, orgID, teamID uuid.UUID) (TeamAISettingsView, error) {
	var view TeamAISettingsView
	err := a.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionTeamUpdate, func(q *tenantdb.Queries) error {
		row, err := q.GetTeamAISettings(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		view = TeamAISettingsView{row.EndpointUrl, row.Model, row.ApiToken != ""}
		return nil
	})
	return view, err
}

func (a *AIApplication) SaveSettings(ctx context.Context, p session.Principal, orgID, teamID uuid.UUID, input TeamAISettingsInput) (TeamAISettingsView, error) {
	var view TeamAISettingsView
	err := a.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionTeamUpdate, func(q *tenantdb.Queries) error {
		input.EndpointURL = strings.TrimSpace(input.EndpointURL)
		input.Model = strings.TrimSpace(input.Model)
		input.APIToken = strings.TrimSpace(input.APIToken)
		endpoint, err := url.Parse(input.EndpointURL)
		if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" ||
			(endpoint.Scheme != "https" && endpoint.Scheme != "http") ||
			len(input.EndpointURL) > 2048 || input.Model == "" || len(input.Model) > 200 || len(input.APIToken) > 8000 {
			return ErrInvalidInput
		}
		if input.APIToken == "" {
			row, err := q.GetTeamAISettings(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidInput
			}
			if err != nil {
				return err
			}
			if row.ApiToken == "" {
				return ErrInvalidInput
			}
		}
		row, err := q.SaveTeamAISettings(ctx, tenantdb.SaveTeamAISettingsParams{EndpointUrl: input.EndpointURL, Model: input.Model, ApiToken: input.APIToken})
		if err != nil {
			return err
		}
		view = TeamAISettingsView{row.EndpointUrl, row.Model, row.ApiToken != ""}
		return nil
	})
	return view, err
}
