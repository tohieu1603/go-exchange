package usecase

import (
	"context"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/trading-service/internal/domain"
)

// Projector is the CQRS read-model writer: it persists trades and order-status
// changes from the events emitted by the matching engine. This service owns the
// orders + trades tables.
type Projector struct {
	orders domain.OrderRepository
	trades domain.TradeRepository
}

func NewProjector(orders domain.OrderRepository, trades domain.TradeRepository) *Projector {
	return &Projector{orders: orders, trades: trades}
}

// ProjectTrade persists an executed trade.
func (p *Projector) ProjectTrade(ctx context.Context, e eventbus.TradeEvent) error {
	return p.trades.Create(ctx, &domain.Trade{
		Pair: e.Pair, BuyOrderID: e.BuyOrderID, SellOrderID: e.SellOrderID,
		BuyerID: e.BuyerID, SellerID: e.SellerID, Price: e.Price, Amount: e.Amount,
		Total: e.Total, BuyerFee: e.BuyerFee, SellerFee: e.SellerFee,
	})
}

// ProjectOrder applies an order-status change to the orders table.
func (p *Projector) ProjectOrder(ctx context.Context, e eventbus.OrderEvent) error {
	return p.orders.UpdateStatus(ctx, e.OrderID, e.Status, e.FilledAmount, e.Price)
}
