package realtime

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultChannel = "corta:chat:events"

// Event is published after a successful chat write so all socket-server
// replicas can fan out to their local WebSocket rooms.
type Event struct {
	TeamID int64  `json:"team_id"`
	Type   string `json:"type"`
	Data   any    `json:"data"`
}

// Publisher pushes chat events onto a Redis Pub/Sub channel.
type Publisher struct {
	client  *redis.Client
	channel string
}

func NewPublisherFromEnv() *Publisher {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	channel := os.Getenv("REDIS_CHAT_CHANNEL")
	if channel == "" {
		channel = defaultChannel
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("realtime: invalid REDIS_URL %q: %v (realtime disabled)", redisURL, err)
		return &Publisher{}
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("realtime: redis ping failed: %v (will retry on publish)", err)
	}

	log.Printf("realtime: publishing chat events to redis channel %q", channel)
	return &Publisher{client: client, channel: channel}
}

// Publish best-effort notifies all socket-server replicas via Redis.
// Failures are logged; the chat REST write remains the source of truth.
func (p *Publisher) Publish(ctx context.Context, event Event) {
	if p == nil || p.client == nil {
		return
	}

	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("realtime: marshal event: %v", err)
		return
	}

	if err := p.client.Publish(ctx, p.channel, body).Err(); err != nil {
		log.Printf("realtime: redis publish failed: %v", err)
	}
}

func (p *Publisher) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}
