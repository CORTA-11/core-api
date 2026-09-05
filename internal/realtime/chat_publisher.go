package realtime

import (
	"context"
	"encoding/json"
	"os"

	"github.com/CORTA-11/core-api/internal/service"
)

const DefaultChatChannel = "corta:chat:events"

type redisPublisher interface {
	Publish(context.Context, string, any) publishCommand
}

type publishCommand interface {
	Err() error
}

type ChatPublisher struct {
	redis   redisPublisher
	channel string
}

func NewChatPublisher(redis redisPublisher, channel string) *ChatPublisher {
	if channel == "" {
		channel = DefaultChatChannel
	}
	return &ChatPublisher{redis: redis, channel: channel}
}

func NewChatPublisherFromEnv(redis redisPublisher) *ChatPublisher {
	return NewChatPublisher(redis, os.Getenv("REDIS_CHAT_CHANNEL"))
}

func (publisher *ChatPublisher) PublishChatEvent(ctx context.Context, event service.ChatEvent) error {
	if publisher == nil || publisher.redis == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return publisher.redis.Publish(ctx, publisher.channel, payload).Err()
}
