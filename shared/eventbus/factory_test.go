package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFromConfig_SelectsBackend(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	// No brokers → Redis Streams backend.
	if _, ok := NewFromConfig(rdb, nil).(*Bus); !ok {
		t.Fatalf("expected *Bus when no brokers configured")
	}
	if _, ok := NewFromConfig(rdb, []string{}).(*Bus); !ok {
		t.Fatalf("expected *Bus for empty broker slice")
	}
	// Brokers present → Kafka backend.
	if _, ok := NewFromConfig(rdb, []string{"localhost:9092"}).(*KafkaBus); !ok {
		t.Fatalf("expected *KafkaBus when brokers configured")
	}
}

func TestDispatch_AckContract(t *testing.T) {
	ctx := context.Background()
	ok := func(context.Context, string, []byte) error { return nil }
	bad := func(context.Context, string, []byte) error { return errors.New("boom") }

	if !dispatch(ctx, []Handler{ok, ok}, "test", "t", "g", "1", nil) {
		t.Fatal("all handlers succeeding must report true (ack)")
	}
	if dispatch(ctx, []Handler{ok, bad}, "test", "t", "g", "1", nil) {
		t.Fatal("any handler erroring must report false (no ack)")
	}
}

// A failing handler must NOT acknowledge the message — it stays pending for the
// consumer group, preserving at-least-once delivery.
func TestRedisBus_FailedHandlerLeavesMessagePending(t *testing.T) {
	b, _ := newBus(t)
	b.Subscribe("orders", func(context.Context, string, []byte) error {
		return errors.New("always fails")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.StartConsumer(ctx, "orders", "proj", "w1")

	require.NoError(t, b.Publish(context.Background(), "orders", map[string]int{"id": 1}))

	assert.Eventually(t, func() bool {
		return b.PendingCount(context.Background(), "orders", "proj") == 1
	}, 3*time.Second, 50*time.Millisecond, "failed message must remain pending (unacked)")
}

// A succeeding handler acknowledges the message — nothing left pending.
func TestRedisBus_SuccessAcksMessage(t *testing.T) {
	b, _ := newBus(t)
	b.Subscribe("orders2", func(context.Context, string, []byte) error { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.StartConsumer(ctx, "orders2", "proj", "w1")

	require.NoError(t, b.Publish(context.Background(), "orders2", map[string]int{"id": 1}))

	assert.Eventually(t, func() bool {
		return b.PendingCount(context.Background(), "orders2", "proj") == 0 &&
			b.StreamLen(context.Background(), "orders2") == 1
	}, 3*time.Second, 50*time.Millisecond, "acked message should leave zero pending")
}
