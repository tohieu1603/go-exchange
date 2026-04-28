package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Event types
const (
	TopicTradeExecuted       = "trade.executed"
	TopicOrderUpdated        = "order.updated"
	TopicBalanceChanged      = "balance.changed"
	TopicPositionChanged     = "position.changed"
	TopicNotificationCreated = "notification.created"
	TopicUserRegistered      = "user.registered"
	TopicPriceUpdated        = "price.updated"
	TopicWSBroadcast         = "ws.broadcast"
	TopicAuditLogged         = "audit.logged"
)

// Handler processes a raw event payload.
type Handler func(ctx context.Context, id string, data []byte) error

// Bus provides event publishing (Redis Streams) and consuming (consumer groups).
type Bus struct {
	rdb      *redis.Client
	handlers map[string][]Handler
}

func New(rdb *redis.Client) *Bus {
	return &Bus{
		rdb:      rdb,
		handlers: make(map[string][]Handler),
	}
}

// Publish sends a typed event to a Redis Stream.
func (b *Bus) Publish(ctx context.Context, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return b.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "stream:" + topic,
		MaxLen: 50000,
		Approx: true,
		Values: map[string]interface{}{
			"topic": topic,
			"data":  string(data),
			"ts":    time.Now().UnixMilli(),
		},
	}).Err()
}

// PublishMulti publishes multiple events in a Redis pipeline (batch, 1 round-trip).
func (b *Bus) PublishMulti(ctx context.Context, events []struct {
	Topic   string
	Payload interface{}
}) error {
	pipe := b.rdb.Pipeline()
	for _, e := range events {
		data, _ := json.Marshal(e.Payload)
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: "stream:" + e.Topic,
			MaxLen: 50000,
			Approx: true,
			Values: map[string]interface{}{
				"topic": e.Topic,
				"data":  string(data),
				"ts":    time.Now().UnixMilli(),
			},
		})
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Subscribe registers a handler for a topic.
func (b *Bus) Subscribe(topic string, handler Handler) {
	b.handlers[topic] = append(b.handlers[topic], handler)
}

// StartConsumer creates a consumer group and starts processing.
// Multiple instances with the same group = load balanced (each event processed once per group).
// Different groups = fan-out (each group gets every event).
func (b *Bus) StartConsumer(ctx context.Context, topic, group, consumer string) {
	stream := "stream:" + topic

	// Create group, ignore if exists
	b.rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			results, err := b.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    group,
				Consumer: consumer,
				Streams:  []string{stream, ">"},
				Count:    50,
				Block:    3 * time.Second,
			}).Result()
			if err != nil {
				if err != redis.Nil {
					time.Sleep(500 * time.Millisecond)
				}
				continue
			}

			for _, result := range results {
				for _, msg := range result.Messages {
					data, _ := msg.Values["data"].(string)
					handlers := b.handlers[topic]
					allOK := true
					for _, h := range handlers {
						if err := h(ctx, msg.ID, []byte(data)); err != nil {
							log.Printf("[eventbus] handler error [%s/%s]: %v", topic, msg.ID, err)
							allOK = false
						}
					}
					if allOK {
						b.rdb.XAck(ctx, stream, group, msg.ID)
					}
				}
			}
		}
	}()

	log.Printf("[eventbus] consumer started: topic=%s group=%s consumer=%s", topic, group, consumer)
}

// PendingCount returns number of unprocessed events for a consumer group.
func (b *Bus) PendingCount(ctx context.Context, topic, group string) int64 {
	info, err := b.rdb.XInfoGroups(ctx, "stream:"+topic).Result()
	if err != nil {
		return -1
	}
	for _, g := range info {
		if g.Name == group {
			return g.Pending
		}
	}
	return -1
}

// PublishWS publishes to Redis Pub/Sub for instant WebSocket fan-out.
func (b *Bus) PublishWS(ctx context.Context, channel string, data interface{}) {
	payload, _ := json.Marshal(map[string]interface{}{
		"channel": channel,
		"data":    data,
	})
	b.rdb.Publish(ctx, "ws:broadcast", string(payload))
}

// StreamLen returns stream length for monitoring.
func (b *Bus) StreamLen(ctx context.Context, topic string) int64 {
	l, _ := b.rdb.XLen(ctx, "stream:"+topic).Result()
	return l
}

// Unmarshal is a helper to unmarshal event data.
func Unmarshal[T any](data []byte) (T, error) {
	var v T
	err := json.Unmarshal(data, &v)
	return v, err
}

// ToMap converts struct to map for backward compat.
func ToMap(v interface{}) map[string]interface{} {
	data, _ := json.Marshal(v)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	return m
}

// Lag returns the consumer lag per topic per group.
func (b *Bus) Lag(ctx context.Context, topic, group string) (pending int64, err error) {
	stream := fmt.Sprintf("stream:%s", topic)
	info, err := b.rdb.XInfoGroups(ctx, stream).Result()
	if err != nil {
		return 0, err
	}
	for _, g := range info {
		if g.Name == group {
			return g.Pending, nil
		}
	}
	return 0, nil
}
