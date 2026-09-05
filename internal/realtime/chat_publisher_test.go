package realtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRedisPublisher struct {
	channel string
	payload any
}

func (publisher *fakeRedisPublisher) Publish(_ context.Context, channel string, payload any) *redis.IntCmd {
	publisher.channel = channel
	publisher.payload = payload
	return redis.NewIntResult(1, nil)
}

func TestChatPublisherPublishesJSONToConfiguredChannel(t *testing.T) {
	t.Parallel()
	redis := &fakeRedisPublisher{}
	publisher := NewChatPublisher(redis, "chat:test")
	event := service.ChatEvent{
		TeamID: 42,
		Type:   "message.created",
		Data:   service.ChatMessageView{ID: uuid.MustParse("11111111-1111-4111-8111-111111111111")},
	}

	require.NoError(t, publisher.PublishChatEvent(context.Background(), event))

	assert.Equal(t, "chat:test", redis.channel)
	payload, ok := redis.payload.([]byte)
	require.True(t, ok)
	var decoded service.ChatEvent
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Equal(t, event, decoded)
}
