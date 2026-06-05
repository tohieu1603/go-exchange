package eventbus

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

// NewFromConfig builds the appropriate EventBus for the deployment:
//   - a Kafka-backed bus when one or more brokers are configured, or
//   - a Redis Streams bus otherwise (single-node / local / demo).
//
// Every service previously inlined the same selection:
//
//	var bus eventbus.EventBus
//	if cfg.UseKafka() {
//	    bus = eventbus.NewKafkaBus(cfg.KafkaBrokerList(), rdb)
//	} else {
//	    bus = eventbus.New(rdb)
//	}
//
// NewFromConfig collapses that to a single call so the construction logic lives
// in one place. It deliberately takes the broker slice (not a config struct) to
// keep the eventbus package free of any dependency on the config package.
func NewFromConfig(rdb *redis.Client, brokers []string) EventBus {
	if len(brokers) > 0 {
		log.Printf("[eventbus] using Kafka backend (brokers=%v)", brokers)
		return NewKafkaBus(brokers, rdb)
	}
	log.Print("[eventbus] using Redis Streams backend")
	return New(rdb)
}

// dispatch runs every handler for a single message and reports whether all of
// them succeeded. A false result means at least one handler errored and the
// caller must NOT acknowledge/commit the message so it gets redelivered.
//
// Both the Redis Streams and Kafka consumers share this so their at-least-once
// delivery semantics are identical: ack only when every handler returns nil.
func dispatch(ctx context.Context, handlers []Handler, backend, topic, group, id string, data []byte) bool {
	allOK := true
	for _, h := range handlers {
		if err := h(ctx, id, data); err != nil {
			log.Printf("[%s] handler error [%s/%s/%s]: %v", backend, group, topic, id, err)
			allOK = false
		}
	}
	return allOK
}

// snapshotHandlers returns a copy of the handlers registered for a topic so a
// consumer goroutine iterates a stable slice that cannot race with any later
// Subscribe call.
func snapshotHandlers(src []Handler) []Handler {
	out := make([]Handler, len(src))
	copy(out, src)
	return out
}
