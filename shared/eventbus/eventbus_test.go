package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBus(t *testing.T) (*Bus, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return New(rdb), mr
}

func TestPublishThenStreamLen(t *testing.T) {
	b, _ := newBus(t)
	require.NoError(t, b.Publish(context.Background(), "test.topic", map[string]string{"k": "v"}))
	require.NoError(t, b.Publish(context.Background(), "test.topic", "x"))
	assert.EqualValues(t, 2, b.StreamLen(context.Background(), "test.topic"))
}

func TestPublishMulti_BatchedInOnePipeline(t *testing.T) {
	b, _ := newBus(t)
	events := []struct {
		Topic   string
		Payload interface{}
	}{
		{"t1", "a"}, {"t1", "b"}, {"t2", "c"},
	}
	require.NoError(t, b.PublishMulti(context.Background(), events))
	assert.EqualValues(t, 2, b.StreamLen(context.Background(), "t1"))
	assert.EqualValues(t, 1, b.StreamLen(context.Background(), "t2"))
}

func TestSubscribeAndConsume_HandlerReceivesPayload(t *testing.T) {
	b, _ := newBus(t)

	var (
		mu       sync.Mutex
		received [][]byte
	)
	b.Subscribe("ride.events", func(_ context.Context, _ string, data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, data)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.StartConsumer(ctx, "ride.events", "test-group", "worker-1")

	require.NoError(t, b.Publish(context.Background(), "ride.events", map[string]int{"x": 1}))
	require.NoError(t, b.Publish(context.Background(), "ride.events", map[string]int{"x": 2}))

	// Consumer has Block=3s; 1s is enough for it to drain XADD'd messages.
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 2
	}, 3*time.Second, 50*time.Millisecond, "expected 2 events delivered")
}

func TestUnmarshalGeneric(t *testing.T) {
	type sample struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	out, err := Unmarshal[sample]([]byte(`{"a":7,"b":"hi"}`))
	require.NoError(t, err)
	assert.Equal(t, 7, out.A)
	assert.Equal(t, "hi", out.B)
}

func TestToMap_RoundtripsStruct(t *testing.T) {
	type x struct {
		Name string `json:"name"`
		Qty  int    `json:"qty"`
	}
	m := ToMap(x{Name: "alice", Qty: 3})
	assert.Equal(t, "alice", m["name"])
	assert.EqualValues(t, 3, m["qty"])
}
