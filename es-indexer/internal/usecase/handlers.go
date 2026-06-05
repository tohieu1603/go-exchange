package usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/shared/eventbus"
)

// Handlers consume domain events and project them into search indices. Each
// Handle* method early-returns nil on a JSON unmarshal failure so a malformed
// event cannot poison the consumer group into an infinite redeliver loop.
type Handlers struct {
	idx Indexer
}

func NewHandlers(idx Indexer) *Handlers { return &Handlers{idx: idx} }

func (h *Handlers) HandleTrade(ctx context.Context, id string, data []byte) error {
	e, err := eventbus.Unmarshal[eventbus.TradeEvent](data)
	if err != nil {
		return nil
	}
	doc := TradeDoc{
		Pair: e.Pair, BuyerID: e.BuyerID, SellerID: e.SellerID,
		Price: e.Price, Amount: e.Amount, Total: e.Total, Side: e.Side, CreatedAt: time.Now(),
	}
	return h.index(ctx, "trades", fmt.Sprintf("trade-%s", id), doc)
}

func (h *Handlers) HandleOrder(ctx context.Context, id string, data []byte) error {
	e, err := eventbus.Unmarshal[eventbus.OrderEvent](data)
	if err != nil {
		return nil
	}
	doc := OrderDoc{
		OrderID: e.OrderID, UserID: e.UserID, Pair: e.Pair, Side: e.Side, Type: e.Type,
		Status: e.Status, Price: e.Price, Amount: e.Amount, FilledAmount: e.FilledAmount, UpdatedAt: time.Now(),
	}
	return h.index(ctx, "orders", fmt.Sprintf("order-%d", e.OrderID), doc)
}

func (h *Handlers) HandleBalance(ctx context.Context, id string, data []byte) error {
	e, err := eventbus.Unmarshal[eventbus.BalanceEvent](data)
	if err != nil {
		return nil
	}
	doc := BalanceDoc{UserID: e.UserID, Currency: e.Currency, Delta: e.Delta, Reason: e.Reason, CreatedAt: time.Now()}
	return h.index(ctx, "balances", fmt.Sprintf("bal-%s", id), doc)
}

func (h *Handlers) HandleAudit(ctx context.Context, id string, data []byte) error {
	e, err := eventbus.Unmarshal[eventbus.AuditLogEvent](data)
	if err != nil {
		return nil
	}
	doc := AuditDoc{
		UserID: e.UserID, Email: e.Email, Action: e.Action, Outcome: e.Outcome,
		IP: e.IP, UserAgent: e.UserAgent, DeviceID: e.DeviceID, NewDevice: e.NewDevice,
		Detail: e.Detail, Timestamp: time.UnixMilli(e.Timestamp),
	}
	return h.index(ctx, "audit_logs", fmt.Sprintf("audit-%s", id), doc)
}

func (h *Handlers) HandleNotification(ctx context.Context, id string, data []byte) error {
	e, err := eventbus.Unmarshal[eventbus.NotificationEvent](data)
	if err != nil {
		return nil
	}
	doc := NotificationDoc{UserID: e.UserID, Type: e.Type, Title: e.Title, Message: e.Message, Pair: e.Pair, CreatedAt: time.Now()}
	return h.index(ctx, "notifications", fmt.Sprintf("notif-%s", id), doc)
}

func (h *Handlers) index(ctx context.Context, index, id string, doc interface{}) error {
	if err := h.idx.Index(ctx, index, id, doc); err != nil {
		log.Printf("[es-indexer] %s index error: %v", index, err)
		return err
	}
	return nil
}
