package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type NotificationPayload struct {
	Title string
	Body  string
	Data  map[string]string
}

type TokenPruner interface {
	DeleteDeviceToken(ctx context.Context, token string) error
}

type Service interface {
	SendMulticast(ctx context.Context, tokens []string, payload NotificationPayload) error
}

type NoopService struct{}

func (n *NoopService) SendMulticast(ctx context.Context, tokens []string, payload NotificationPayload) error {
	return nil
}

type FCMService struct {
	client    *http.Client
	projectID string
	pruner    TokenPruner
	logger    *slog.Logger
}

type fcmRequest struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token        string            `json:"token"`
	Notification *fcmNotification  `json:"notification,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
	Android      *fcmAndroid       `json:"android,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

type fcmAndroid struct {
	Priority     string                  `json:"priority,omitempty"`
	Notification *fcmAndroidNotification `json:"notification,omitempty"`
}

type fcmAndroidNotification struct {
	ChannelID    string `json:"channel_id,omitempty"`
	DefaultSound bool   `json:"default_sound,omitempty"`
}

type fcmErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// NewFCMService creates an FCM push service from credentials JSON bytes.
func NewFCMService(ctx context.Context, credentialsJSON []byte, pruner TokenPruner, logger *slog.Logger) (*FCMService, error) {
	if logger == nil {
		logger = slog.Default()
	}
	creds, err := google.CredentialsFromJSON(ctx, credentialsJSON, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, fmt.Errorf("parse firebase credentials: %w", err)
	}
	if creds.ProjectID == "" {
		return nil, errors.New("firebase credentials missing project_id")
	}

	client := oauth2.NewClient(ctx, creds.TokenSource)
	client.Timeout = 10 * time.Second

	return &FCMService{
		client:    client,
		projectID: creds.ProjectID,
		pruner:    pruner,
		logger:    logger,
	}, nil
}

// NewFCMServiceFromFile creates an FCM push service from a credentials file path.
func NewFCMServiceFromFile(ctx context.Context, filePath string, pruner TokenPruner, logger *slog.Logger) (*FCMService, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read firebase credentials file: %w", err)
	}
	return NewFCMService(ctx, data, pruner, logger)
}

// SendMulticast sends a push notification to all provided device tokens.
func (f *FCMService) SendMulticast(ctx context.Context, tokens []string, payload NotificationPayload) error {
	if len(tokens) == 0 {
		return nil
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", f.projectID)

	var wg sync.WaitGroup
	// Limit concurrency to 10 workers
	sem := make(chan struct{}, 10)

	for _, token := range tokens {
		if token == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}

		go func(t string) {
			defer wg.Done()
			defer func() { <-sem }()

			reqBody := fcmRequest{
				Message: fcmMessage{
					Token: t,
					Notification: &fcmNotification{
						Title: payload.Title,
						Body:  payload.Body,
					},
					Data: payload.Data,
					Android: &fcmAndroid{
						Priority: "HIGH",
						Notification: &fcmAndroidNotification{
							ChannelID:    "default",
							DefaultSound: true,
						},
					},
				},
			}

			jsonBytes, err := json.Marshal(reqBody)
			if err != nil {
				f.logger.Error("failed to marshal FCM request", "error", err)
				return
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
			if err != nil {
				f.logger.Error("failed to create FCM HTTP request", "error", err)
				return
			}
			req.Header.Set("Content-Type", "application/json; UTF-8")

			resp, err := f.client.Do(req)
			if err != nil {
				f.logger.Warn("FCM request error", "token", t, "error", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return
			}

			body, _ := io.ReadAll(resp.Body)
			var errResp fcmErrorResponse
			_ = json.Unmarshal(body, &errResp)

			f.logger.Warn("FCM send failed", "status", resp.StatusCode, "token", t, "error_status", errResp.Error.Status, "message", errResp.Error.Message)

			// Clean up unregistered or invalid tokens
			if resp.StatusCode == http.StatusNotFound || errResp.Error.Status == "UNREGISTERED" || errResp.Error.Status == "NOT_FOUND" {
				if f.pruner != nil {
					pruneCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if pruneErr := f.pruner.DeleteDeviceToken(pruneCtx, t); pruneErr != nil {
						f.logger.Warn("failed to prune stale device token", "token", t, "error", pruneErr)
					} else {
						f.logger.Info("pruned stale device token", "token", t)
					}
				}
			}
		}(token)
	}

	wg.Wait()
	return nil
}
