package redisutil

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_ConnectsAndPings(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb, err := NewClient("redis://" + mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	assert.NoError(t, rdb.Ping(context.Background()).Err())
}

func TestNewClient_BadURL(t *testing.T) {
	_, err := NewClient("not-a-valid-redis-url")
	assert.Error(t, err, "an unparseable URL must return an error, not panic")
}

func TestNewClient_UnreachableFailsPing(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	addr := mr.Addr()
	mr.Close() // server gone — connection now refused

	_, err = NewClient("redis://" + addr)
	assert.Error(t, err, "ping against a closed server must fail fast")
}
