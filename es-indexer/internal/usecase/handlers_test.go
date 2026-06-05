package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every Handle* method early-returns nil on a JSON unmarshal failure so a
// malformed event cannot poison the consumer group into an infinite redeliver
// loop. The bad-JSON branch returns before the Indexer is touched, so a nil
// indexer is fine here.
func TestHandlers_BadJSON_ReturnsNilNotError(t *testing.T) {
	h := &Handlers{idx: nil}
	bad := []byte("not-json{{")

	assert.NoError(t, h.HandleTrade(context.Background(), "x", bad))
	assert.NoError(t, h.HandleOrder(context.Background(), "x", bad))
	assert.NoError(t, h.HandleBalance(context.Background(), "x", bad))
	assert.NoError(t, h.HandleAudit(context.Background(), "x", bad))
	assert.NoError(t, h.HandleNotification(context.Background(), "x", bad))
}

// On a valid event the handler forwards the projected doc to the Indexer port.
func TestHandlers_ValidTrade_Indexes(t *testing.T) {
	fake := &fakeIndexer{}
	h := NewHandlers(fake)
	if err := h.HandleTrade(context.Background(), "1-1", []byte(`{"pair":"BTC_USDT","price":50000,"amount":1,"side":"BUY"}`)); err != nil {
		t.Fatalf("HandleTrade: %v", err)
	}
	if fake.calls != 1 || fake.lastIndex != "trades" {
		t.Fatalf("expected one index into 'trades', got %d into %q", fake.calls, fake.lastIndex)
	}
}

type fakeIndexer struct {
	calls     int
	lastIndex string
}

func (f *fakeIndexer) Index(_ context.Context, index, _ string, _ interface{}) error {
	f.calls++
	f.lastIndex = index
	return nil
}
