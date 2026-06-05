package usecase

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/trading-service/internal/domain"
)

// MatchingEngine is the CQRS command handler for order processing: in-memory
// order book + atomic Redis Lua balance ops + event publishing. Zero DB writes
// on the critical path (persistence is async via the Projector).
type MatchingEngine struct {
	books        map[string]*domain.OrderBook
	orderRepo    domain.OrderRepository
	balCache     BalanceCache
	locker       *BalanceLocker
	marketClient PriceGateway
	bus          eventbus.EventPublisher
	fees         FeeResolver
	mu           sync.RWMutex
	pairLocks    map[string]*sync.Mutex
}

func NewMatchingEngine(orderRepo domain.OrderRepository, balCache BalanceCache, locker *BalanceLocker, marketClient PriceGateway, bus eventbus.EventPublisher, fees FeeResolver) *MatchingEngine {
	if fees == nil {
		fees = NewFlatFeeResolver(0.001, 0.001)
	}
	me := &MatchingEngine{
		books: make(map[string]*domain.OrderBook), orderRepo: orderRepo, balCache: balCache,
		locker: locker, marketClient: marketClient, bus: bus, fees: fees,
		pairLocks: make(map[string]*sync.Mutex),
	}
	for _, coin := range types.DefaultCoins {
		pair := coin.Symbol + "_USDT"
		me.books[pair] = domain.NewOrderBook(pair)
		me.pairLocks[pair] = &sync.Mutex{}
	}
	return me
}

// LoadOpenOrders rebuilds the in-memory books from persisted open LIMIT orders.
func (me *MatchingEngine) LoadOpenOrders() {
	orders, err := me.orderRepo.FindOpenLimitOrders(context.Background())
	if err != nil {
		log.Printf("LoadOpenOrders error: %v", err)
		return
	}
	for i := range orders {
		if book, ok := me.books[orders[i].Pair]; ok {
			book.AddOrder(&orders[i])
		}
	}
	log.Printf("Loaded %d open LIMIT orders into order books", len(orders))
}

func (me *MatchingEngine) updateBalanceRedis(ctx context.Context, userID uint, currency string, delta float64) error {
	if delta == 0 {
		return nil
	}
	if delta < 0 {
		_, err := me.balCache.Deduct(ctx, userID, currency, -delta)
		return err
	}
	_, err := me.balCache.Credit(ctx, userID, currency, delta)
	return err
}

func (me *MatchingEngine) unlockBalanceRedis(ctx context.Context, userID uint, currency string, amount float64) {
	if me.locker != nil {
		me.locker.Unlock(ctx, userID, currency, amount)
		return
	}
	_ = me.balCache.Unlock(ctx, userID, currency, amount)
}

// ProcessOrder matches → settles balances via Redis Lua → publishes events.
func (me *MatchingEngine) ProcessOrder(order *domain.Order) error {
	me.mu.RLock()
	book, ok := me.books[order.Pair]
	pairLock := me.pairLocks[order.Pair]
	me.mu.RUnlock()
	if !ok {
		return nil
	}
	parts := strings.Split(order.Pair, "_")
	if len(parts) != 2 {
		return nil
	}
	base, quote := parts[0], parts[1]
	ctx := context.Background()

	pairLock.Lock()
	defer pairLock.Unlock()

	metrics.OrdersPlaced.WithLabelValues(order.Pair, order.Side, order.Type).Inc()

	trades := book.Match(order)

	type event struct {
		Topic   string
		Payload interface{}
	}
	var events []event
	settledCount := 0

	for _, t := range trades {
		total := t.Price * t.Amount
		metrics.TradeVolumeUSDT.Add(total)

		buyerIsTaker := order.Side == domain.SideBuy
		var buyerFee, sellerFee float64
		buyerMaker, buyerTaker := me.fees.Rates(t.BuyOrder.UserID)
		sellerMaker, sellerTaker := me.fees.Rates(t.SellOrder.UserID)
		if buyerIsTaker {
			buyerFee = total * buyerTaker
			sellerFee = total * sellerMaker
		} else {
			buyerFee = total * buyerMaker
			sellerFee = total * sellerTaker
		}
		totalFee := buyerFee + sellerFee

		if err := me.updateBalanceRedis(ctx, t.BuyOrder.UserID, quote, -(total + buyerFee)); err != nil {
			me.rollbackTrades(order, book, trades, settledCount)
			return err
		}
		if err := me.updateBalanceRedis(ctx, t.BuyOrder.UserID, base, t.Amount); err != nil {
			me.updateBalanceRedis(ctx, t.BuyOrder.UserID, quote, total+buyerFee) //nolint:errcheck
			me.rollbackTrades(order, book, trades, settledCount)
			return err
		}
		if err := me.updateBalanceRedis(ctx, t.SellOrder.UserID, base, -t.Amount); err != nil {
			me.updateBalanceRedis(ctx, t.BuyOrder.UserID, quote, total+buyerFee) //nolint:errcheck
			me.updateBalanceRedis(ctx, t.BuyOrder.UserID, base, -t.Amount)        //nolint:errcheck
			me.rollbackTrades(order, book, trades, settledCount)
			return err
		}
		if err := me.updateBalanceRedis(ctx, t.SellOrder.UserID, quote, total-sellerFee); err != nil {
			me.updateBalanceRedis(ctx, t.BuyOrder.UserID, quote, total+buyerFee) //nolint:errcheck
			me.updateBalanceRedis(ctx, t.BuyOrder.UserID, base, -t.Amount)        //nolint:errcheck
			me.updateBalanceRedis(ctx, t.SellOrder.UserID, base, t.Amount)        //nolint:errcheck
			me.rollbackTrades(order, book, trades, settledCount)
			return err
		}
		if feeID := PlatformFeeUserID(); totalFee > 0 && feeID > 0 {
			me.updateBalanceRedis(ctx, feeID, quote, totalFee) //nolint:errcheck
			events = append(events, event{Topic: eventbus.TopicBalanceChanged, Payload: eventbus.BalanceEvent{
				UserID: feeID, Currency: quote, Delta: totalFee, Reason: "fee",
			}})
		}

		settledCount++

		if t.BuyOrder.Type == domain.TypeLimit {
			me.unlockBalanceRedis(ctx, t.BuyOrder.UserID, quote, total+buyerFee)
		}
		if t.SellOrder.Type == domain.TypeLimit {
			me.unlockBalanceRedis(ctx, t.SellOrder.UserID, base, t.Amount)
		}

		for _, be := range []eventbus.BalanceEvent{
			{UserID: t.BuyOrder.UserID, Currency: quote, Delta: -(total + buyerFee), Reason: "trade"},
			{UserID: t.BuyOrder.UserID, Currency: base, Delta: t.Amount, Reason: "trade"},
			{UserID: t.SellOrder.UserID, Currency: base, Delta: -t.Amount, Reason: "trade"},
			{UserID: t.SellOrder.UserID, Currency: quote, Delta: total - sellerFee, Reason: "trade"},
		} {
			events = append(events, event{Topic: eventbus.TopicBalanceChanged, Payload: be})
		}

		events = append(events,
			event{Topic: eventbus.TopicTradeExecuted, Payload: eventbus.TradeEvent{
				Pair: order.Pair, BuyOrderID: t.BuyOrder.ID, SellOrderID: t.SellOrder.ID,
				BuyerID: t.BuyOrder.UserID, SellerID: t.SellOrder.UserID,
				Price: t.Price, Amount: t.Amount, Total: total,
				BuyerFee: buyerFee, SellerFee: sellerFee, Side: order.Side,
			}},
			event{Topic: eventbus.TopicOrderUpdated, Payload: orderEvent(t.BuyOrder, order.Pair)},
			event{Topic: eventbus.TopicOrderUpdated, Payload: orderEvent(t.SellOrder, order.Pair)},
		)

		me.bus.PublishWS(ctx, "trades@"+order.Pair, map[string]interface{}{
			"price": t.Price, "amount": t.Amount, "side": order.Side,
		})
	}

	// MARKET instant fill (demo) at realtime price; aggressor pays taker fee.
	if order.Type == domain.TypeMarket && !order.IsFilled() {
		remaining := order.Remaining()
		marketPrice, _ := me.marketClient.GetPrice(ctx, order.Pair)
		if marketPrice > 0 && remaining > 0 {
			total := marketPrice * remaining
			_, takerRate := me.fees.Rates(order.UserID)
			fee := total * takerRate

			var balDeltas []eventbus.BalanceEvent
			if order.Side == domain.SideBuy {
				if err := me.updateBalanceRedis(ctx, order.UserID, quote, -(total + fee)); err != nil {
					me.rollbackTrades(order, book, trades, settledCount)
					return err
				}
				if err := me.updateBalanceRedis(ctx, order.UserID, base, remaining); err != nil {
					me.updateBalanceRedis(ctx, order.UserID, quote, total+fee) //nolint:errcheck
					me.rollbackTrades(order, book, trades, settledCount)
					return err
				}
				balDeltas = []eventbus.BalanceEvent{
					{UserID: order.UserID, Currency: quote, Delta: -(total + fee), Reason: "trade"},
					{UserID: order.UserID, Currency: base, Delta: remaining, Reason: "trade"},
				}
			} else {
				if err := me.updateBalanceRedis(ctx, order.UserID, base, -remaining); err != nil {
					me.rollbackTrades(order, book, trades, settledCount)
					return err
				}
				if err := me.updateBalanceRedis(ctx, order.UserID, quote, total-fee); err != nil {
					me.updateBalanceRedis(ctx, order.UserID, base, remaining) //nolint:errcheck
					me.rollbackTrades(order, book, trades, settledCount)
					return err
				}
				balDeltas = []eventbus.BalanceEvent{
					{UserID: order.UserID, Currency: base, Delta: -remaining, Reason: "trade"},
					{UserID: order.UserID, Currency: quote, Delta: total - fee, Reason: "trade"},
				}
			}

			if feeID := PlatformFeeUserID(); fee > 0 && feeID > 0 {
				me.updateBalanceRedis(ctx, feeID, quote, fee) //nolint:errcheck
				events = append(events, event{Topic: eventbus.TopicBalanceChanged, Payload: eventbus.BalanceEvent{
					UserID: feeID, Currency: quote, Delta: fee, Reason: "fee",
				}})
			}

			order.Price = marketPrice
			order.FilledAmount = order.Amount
			order.Status = domain.StatusFilled

			buyID, sellID := order.ID, uint(0)
			buyerID, sellerID := order.UserID, uint(0)
			if order.Side == domain.SideSell {
				buyID, sellID = 0, order.ID
				buyerID, sellerID = 0, order.UserID
			}
			events = append(events, event{Topic: eventbus.TopicTradeExecuted, Payload: eventbus.TradeEvent{
				Pair: order.Pair, BuyOrderID: buyID, SellOrderID: sellID,
				BuyerID: buyerID, SellerID: sellerID,
				Price: marketPrice, Amount: remaining, Total: total, BuyerFee: fee, Side: order.Side,
			}})
			for _, be := range balDeltas {
				events = append(events, event{Topic: eventbus.TopicBalanceChanged, Payload: be})
			}
			me.bus.PublishWS(ctx, "trades@"+order.Pair, map[string]interface{}{
				"price": marketPrice, "amount": remaining, "side": order.Side,
			})
		}
	}

	if !order.IsFilled() && order.Type == domain.TypeLimit {
		book.AddOrder(order)
	}

	events = append(events, event{Topic: eventbus.TopicOrderUpdated, Payload: orderEvent(order, order.Pair)})
	for _, e := range events {
		me.bus.Publish(ctx, e.Topic, e.Payload)
	}
	me.bus.PublishWS(ctx, "depth@"+order.Pair, book.GetDepth(20))
	metrics.OrderBookSize.WithLabelValues(order.Pair).Set(float64(bookOrderCount(book)))
	return nil
}

func orderEvent(o *domain.Order, pair string) eventbus.OrderEvent {
	return eventbus.OrderEvent{
		OrderID: o.ID, UserID: o.UserID, Pair: pair, Side: o.Side, Type: o.Type,
		Price: o.Price, Amount: o.Amount, FilledAmount: o.FilledAmount, Status: o.Status,
	}
}

func bookOrderCount(book *domain.OrderBook) int {
	d := book.GetDepth(1000)
	return len(d.Bids) + len(d.Asks)
}

// rollbackTrades reverts orderbook state for unsettled trades only.
func (me *MatchingEngine) rollbackTrades(order *domain.Order, book *domain.OrderBook, trades []domain.TradeResult, settledCount int) {
	for i := settledCount; i < len(trades); i++ {
		t := trades[i]
		t.BuyOrder.FilledAmount -= t.Amount
		t.SellOrder.FilledAmount -= t.Amount
		if t.BuyOrder.FilledAmount <= 0 {
			t.BuyOrder.Status = domain.StatusOpen
		} else {
			t.BuyOrder.Status = domain.StatusPartial
		}
		if t.SellOrder.FilledAmount <= 0 {
			t.SellOrder.Status = domain.StatusOpen
		} else {
			t.SellOrder.Status = domain.StatusPartial
		}
		if order.Side == domain.SideBuy {
			if t.SellOrder.Status == domain.StatusOpen {
				book.AddOrder(t.SellOrder)
			}
		} else {
			if t.BuyOrder.Status == domain.StatusOpen {
				book.AddOrder(t.BuyOrder)
			}
		}
	}
	var filled float64
	for i := 0; i < settledCount; i++ {
		filled += trades[i].Amount
	}
	order.FilledAmount = filled
	if filled <= 0 {
		order.Status = domain.StatusOpen
	} else {
		order.Status = domain.StatusPartial
	}
}

// CancelOrder removes an order from the book and broadcasts the update.
func (me *MatchingEngine) CancelOrder(order *domain.Order) {
	me.mu.RLock()
	book, ok := me.books[order.Pair]
	me.mu.RUnlock()
	if !ok {
		return
	}
	book.RemoveOrder(order.ID, order.Side)
	ctx := context.Background()
	me.bus.Publish(ctx, eventbus.TopicOrderUpdated, eventbus.OrderEvent{
		OrderID: order.ID, UserID: order.UserID, Pair: order.Pair, Side: order.Side, Type: order.Type,
		Price: order.Price, Amount: order.Amount, FilledAmount: order.FilledAmount, Status: domain.StatusCancelled,
	})
	me.bus.PublishWS(ctx, "depth@"+order.Pair, book.GetDepth(20))
}

// GetCurrentPrice returns the live market price for a pair via the price gateway.
func (me *MatchingEngine) GetCurrentPrice(pair string) float64 {
	price, _ := me.marketClient.GetPrice(context.Background(), pair)
	return price
}

// GetDepth returns the in-memory book depth (CQRS query).
func (me *MatchingEngine) GetDepth(pair string, limit int) domain.DepthData {
	me.mu.RLock()
	book, ok := me.books[pair]
	me.mu.RUnlock()
	if !ok {
		return domain.DepthData{}
	}
	return book.GetDepth(limit)
}
