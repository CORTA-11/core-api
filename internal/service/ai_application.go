package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	maxAIContextMessages = 100
	maxAIResponseBytes   = 2 << 20
)

type AIProviderConfig struct {
	Protocol         string `json:"protocol"`
	EndpointURL      string `json:"endpoint_url"`
	Model            string `json:"model"`
	APIToken         string `json:"api_token"`
	StructuredOutput bool   `json:"structured_output"`
}

type AIProcessOptions struct {
	ResponseLanguage string `json:"response_language"`
	MaxActionItems   int    `json:"max_action_items"`
}

type AIProcessInput struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type AIContextParticipant struct {
	PublicUserID string `json:"public_user_id"`
	DisplayName  string `json:"display_name"`
}

type AIContextMessage struct {
	PublicMessageID    string    `json:"public_message_id"`
	SenderPublicUserID string    `json:"sender_public_user_id"`
	Timestamp          time.Time `json:"timestamp"`
	Text               string    `json:"text"`
}

type AIProcessRequest struct {
	SchemaVersion string           `json:"schema_version"`
	RequestID     uuid.UUID        `json:"request_id"`
	Operation     string           `json:"operation"`
	Provider      AIProviderConfig `json:"provider"`
	Context       AIContext        `json:"context"`
	Options       AIProcessOptions `json:"options"`
}

type AIContext struct {
	OrganizationPublicID string                 `json:"organization_public_id"`
	TeamPublicID         string                 `json:"team_public_id"`
	ReferenceTimestamp   time.Time              `json:"reference_timestamp"`
	Timezone             string                 `json:"timezone"`
	Participants         []AIContextParticipant `json:"participants"`
	Messages             []AIContextMessage     `json:"messages"`
}

type AIDecision struct {
	Text             string   `json:"text"`
	SourceMessageIDs []string `json:"source_message_ids"`
}

type AISummary struct {
	Overview      string       `json:"overview"`
	KeyPoints     []string     `json:"key_points"`
	Decisions     []AIDecision `json:"decisions"`
	OpenQuestions []string     `json:"open_questions"`
}

type AICandidateActionItem struct {
	CandidateID      uuid.UUID  `json:"candidate_id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	AssigneeUserID   *uuid.UUID `json:"assignee_user_id"`
	Priority         *string    `json:"priority"`
	DueDate          *string    `json:"due_date"`
	SourceMessageIDs []string   `json:"source_message_ids"`
	Confidence       float64    `json:"confidence"`
}

type AIProcessResult struct {
	SchemaVersion string                  `json:"schema_version"`
	RequestID     uuid.UUID               `json:"request_id"`
	Summary       AISummary               `json:"summary"`
	ActionItems   []AICandidateActionItem `json:"action_items"`
}

type AIClient interface {
	Process(context.Context, AIProcessRequest) (AIProcessResult, error)
}

type AIApplication struct {
	authorizer applicationAuthorizer
	client     AIClient
}

func NewAIApplication(authorizer applicationAuthorizer, client AIClient) *AIApplication {
	return &AIApplication{authorizer: authorizer, client: client}
}

func (application *AIApplication) Process(ctx context.Context, principal session.Principal, orgID, teamID uuid.UUID, input AIProcessInput) (AIProcessResult, error) {
	if application == nil || application.authorizer == nil || application.client == nil {
		return AIProcessResult{}, ErrDependencyUnavailable
	}
	if input.From.IsZero() || input.To.IsZero() || input.From.After(input.To) {
		return AIProcessResult{}, ErrInvalidInput
	}
	var provider AIProviderConfig
	var contextData AIContext
	err := application.authorizer.WithinTeam(ctx, principal, orgID, teamID, authorization.PermissionRealtimeConnect, func(queries *tenantdb.Queries) error {
		settings, err := queries.GetTeamAISettings(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAISettingsMissing
		}
		if err != nil {
			return err
		}
		provider = AIProviderConfig{Protocol: "openai_chat_completions_v1", EndpointURL: settings.EndpointUrl, Model: settings.Model, APIToken: settings.ApiToken, StructuredOutput: true}
		members, err := queries.ListBoundTeamMembers(ctx, 10000)
		if err != nil {
			return err
		}
		contextData.Participants = make([]AIContextParticipant, 0, len(members))
		for _, member := range members {
			contextData.Participants = append(contextData.Participants, AIContextParticipant{
				PublicUserID: member.UserPublicID.String(), DisplayName: member.DisplayName,
			})
		}
		rows, err := queries.ListAIChatMessages(ctx, tenantdb.ListAIChatMessagesParams{FromTime: input.From, ToTime: input.To, Limit: maxAIContextMessages + 1})
		if err != nil {
			return err
		}
		if len(rows) > maxAIContextMessages {
			return ErrInvalidInput
		}
		contextData.Messages = make([]AIContextMessage, 0, len(rows))
		for index := 0; index < len(rows); index++ {
			row := rows[index]
			if strings.TrimSpace(row.Message) == "" {
				continue
			}
			contextData.Messages = append(contextData.Messages, AIContextMessage{
				PublicMessageID: row.PublicID.String(), SenderPublicUserID: row.SenderUserPublicID.String(),
				Timestamp: row.CreatedAt, Text: row.Message,
			})
		}
		contextData.OrganizationPublicID = orgID.String()
		contextData.TeamPublicID = teamID.String()
		contextData.ReferenceTimestamp = time.Now().UTC()
		contextData.Timezone = "UTC"
		return nil
	})
	if err != nil {
		return AIProcessResult{}, err
	}
	if len(contextData.Messages) == 0 {
		return AIProcessResult{}, ErrInvalidInput
	}
	return application.client.Process(ctx, AIProcessRequest{
		SchemaVersion: "1", RequestID: uuid.New(), Operation: "chat_summary_and_actions",
		Provider: provider, Context: contextData, Options: AIProcessOptions{ResponseLanguage: "en", MaxActionItems: 10},
	})
}

type HTTPAIClient struct {
	baseURL       string
	internalToken string
	client        *http.Client
}

func NewHTTPAIClient(baseURL, internalToken string, timeout time.Duration) (*HTTPAIClient, error) {
	if strings.TrimSpace(baseURL) == "" || timeout <= 0 {
		return nil, errors.New("AI service URL and timeout are required")
	}
	return &HTTPAIClient{baseURL: strings.TrimRight(baseURL, "/"), internalToken: internalToken, client: &http.Client{Timeout: timeout}}, nil
}

func (client *HTTPAIClient) Process(ctx context.Context, request AIProcessRequest) (AIProcessResult, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return AIProcessResult{}, ErrInvalidInput
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/v1/process", bytes.NewReader(body))
	if err != nil {
		return AIProcessResult{}, ErrDependencyUnavailable
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if client.internalToken != "" {
		httpRequest.Header.Set("X-Synodus-AI-Token", client.internalToken)
	}
	response, err := client.client.Do(httpRequest)
	if err != nil {
		return AIProcessResult{}, ErrDependencyUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	limited := io.LimitReader(response.Body, maxAIResponseBytes)
	responseBody, err := io.ReadAll(limited)
	if err != nil || len(responseBody) == maxAIResponseBytes {
		return AIProcessResult{}, ErrDependencyUnavailable
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return AIProcessResult{}, aiProviderError(response.StatusCode, responseBody)
	}
	var result AIProcessResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return AIProcessResult{}, ErrDependencyUnavailable
	}
	return result, nil
}

func aiProviderError(status int, body []byte) error {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.Error.Code == "AI_INVALID_REQUEST" || envelope.Error.Code == "AI_ENDPOINT_NOT_ALLOWED" || envelope.Error.Code == "AI_PROVIDER_PROTOCOL_UNSUPPORTED" {
		return ErrInvalidInput
	}
	if status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout {
		return ErrDependencyUnavailable
	}
	return ErrDependencyUnavailable
}

var ErrDependencyUnavailable = errors.New("AI dependency unavailable")
