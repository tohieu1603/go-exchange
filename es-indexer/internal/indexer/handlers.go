package indexer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/shared/eventbus"
)

// Handlers consumes Kafka events and indexes them into Elasticsearch
type Handlers struct {
	es *ESClient
}

func NewHandlers(es *ESClient) *Handlers {
	return &Handlers{es: es}
}

func (h *Handlers) HandleTrade(_ context.Context, id string, data []byte) error {
	event, err := eventbus.Unmarshal[eventbus.TradeEvent](data)
	if err != nil {
		return nil
	}
	doc := TradeDoc{
		Pair: event.Pair, BuyerID: event.BuyerID, SellerID: event.SellerID,
		Price: event.Price, Amount: event.Amount, Total: event.Total,
		Side: event.Side, CreatedAt: time.Now(),
	}
	if err := h.es.Index(context.Background(), "trades", fmt.Sprintf("trade-%s", id), doc); err != nil {
		log.Printf("[es-indexer] trade index error: %v", err)
		return err
	}
	return nil
}

func (h *Handlers) HandleOrder(_ context.Context, id string, data []byte) error {
	event, err := eventbus.Unmarshal[eventbus.OrderEvent](data)
	if err != nil {
		return nil
	}
	doc := OrderDoc{
		OrderID: event.OrderID, UserID: event.UserID, Pair: event.Pair,
		Side: event.Side, Type: event.Type, Status: event.Status,
		Price: event.Price, Amount: event.Amount, FilledAmount: event.FilledAmount,
		UpdatedAt: time.Now(),
	}
	if err := h.es.Index(context.Background(), "orders", fmt.Sprintf("order-%d", event.OrderID), doc); err != nil {
		log.Printf("[es-indexer] order index error: %v", err)
		return err
	}
	return nil
}

func (h *Handlers) HandleBalance(_ context.Context, id string, data []byte) error {
	event, err := eventbus.Unmarshal[eventbus.BalanceEvent](data)
	if err != nil {
		return nil
	}
	doc := BalanceDoc{
		UserID: event.UserID, Currency: event.Currency,
		Delta: event.Delta, Reason: event.Reason,
		CreatedAt: time.Now(),
	}
	if err := h.es.Index(context.Background(), "balances", fmt.Sprintf("bal-%s", id), doc); err != nil {
		log.Printf("[es-indexer] balance index error: %v", err)
		return err
	}
	return nil
}

// HandleAudit indexes security audit rows into the "audit_logs" ES index.
// Includes device fingerprint + new-device flag so Kibana can power
// security dashboards (failed-login heatmap, new-device timelines, etc.).
func (h *Handlers) HandleAudit(_ context.Context, id string, data []byte) error {
	event, err := eventbus.Unmarshal[eventbus.AuditLogEvent](data)
	if err != nil {
		return nil
	}
	doc := AuditDoc{
		UserID: event.UserID, Email: event.Email,
		Action: event.Action, Outcome: event.Outcome,
		IP: event.IP, UserAgent: event.UserAgent,
		DeviceID: event.DeviceID, NewDevice: event.NewDevice,
		Detail:    event.Detail,
		Timestamp: time.UnixMilli(event.Timestamp),
	}
	if err := h.es.Index(context.Background(), "audit_logs", fmt.Sprintf("audit-%s", id), doc); err != nil {
		log.Printf("[es-indexer] audit index error: %v", err)
		return err
	}
	return nil
}

func (h *Handlers) HandleNotification(_ context.Context, id string, data []byte) error {
	event, err := eventbus.Unmarshal[eventbus.NotificationEvent](data)
	if err != nil {
		return nil
	}
	doc := NotificationDoc{
		UserID: event.UserID, Type: event.Type, Title: event.Title,
		Message: event.Message, Pair: event.Pair, CreatedAt: time.Now(),
	}
	if err := h.es.Index(context.Background(), "notifications", fmt.Sprintf("notif-%s", id), doc); err != nil {
		log.Printf("[es-indexer] notification index error: %v", err)
		return err
	}
	return nil
}
