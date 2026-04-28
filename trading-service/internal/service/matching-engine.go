package service

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/trading-service/internal/model"
	"github.com/cryptox/trading-service/internal/engine"
	grpcclient "github.com/cryptox/trading-service/internal/grpc"
	"github.com/cryptox/trading-service/internal/repository"
)

// MatchingEngine is the CQRS command handler for order processing.
// Hot path: in-memory orderbook + Redis Lua balance + event publishing.
// Zero DB writes on the critical path.
type MatchingEngine struct {
	books        map[string]*engine.OrderBook
	orderRepo    repository.OrderRepo
	balCache     *redisutil.BalanceCache
	locker       *BalanceLocker
	walletClient *grpcclient.WalletClient
	marketClient *grpcclient.MarketClient
	bus          eventbus.EventPublisher
	fees         FeeResolver
	mu           sync.RWMutex
	pairLocks    map[string]*sync.Mutex
}

func NewMatchingEngine(
	orderRepo repository.OrderRepo,
	balCache *redisutil.BalanceCache,
	locker *BalanceLocker,
	walletClient *grpcclient.WalletClient,
	marketClient *grpcclient.MarketClient,
	bus eventbus.EventPublisher,
	fees FeeResolver,
) *MatchingEngine {
	if fees == nil {
		fees = NewFlatFeeResolver(0.001, 0.001) // safe default
	}
	me := &MatchingEngine{
		books:        make(map[string]*engine.OrderBook),
		orderRepo:    orderRepo,
		balCache:     balCache,
		locker:       locker,
		walletClient: walletClient,
		marketClient: marketClient,
		bus:          bus,
		fees:         fees,
		pairLocks:    make(map[string]*sync.Mutex),
	}
	for _, coin := range types.DefaultCoins {
		pair := coin.Symbol + "_USDT"
		me.books[pair] = engine.NewOrderBook(pair)
		me.pairLocks[pair] = &sync.Mutex{}
	}
	return me
}

// LoadOpenOrders cold-start recovery from DB (runs once on startup).
func (me *MatchingEngine) LoadOpenOrders() {
	orders, err := me.orderRepo.FindOpenLimitOrders()
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

// updateBalanceRedis applies a balance delta via Redis Lua (hot path).
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
	me.balCache.Unlock(ctx, userID, currency, amount) //nolint:errcheck
}

// ProcessOrder is the CQRS command handler.
// Flow: match → Redis Lua balance (atomic) → publish events → return.
// DB persistence handled async by projectors consuming events.
func (me *MatchingEngine) ProcessOrder(order *model.Order) error {
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

	// Per-pair lock: serialize matching for this pair
	pairLock.Lock()
	defer pairLock.Unlock()

	// Order count metric — labelled by pair/side/type for slicing.
	metrics.OrdersPlaced.WithLabelValues(order.Pair, order.Side, order.Type).Inc()

	// Step 1: Match against orderbook (pure in-memory)
	trades := book.Match(order)

	// Step 2: Settle matched trades via Redis Lua (atomic balance ops)
	type event struct {
		Topic   string
		Payload interface{}
	}
	var events []event

	settledCount := 0
	for _, t := range trades {
		total := t.Price * t.Amount
		metrics.TradeVolumeUSDT.Add(total)

		// Maker = the resting order; Taker = the incoming aggressor.
		// In OrderBook.Match the incoming is always `order` itself.
		buyerIsTaker := order.Side == "BUY"
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

		// Redis Lua atomic transfers — rollback only unsettled trades on failure.
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
		// Credit collected fees to platform fee wallet (preserves balance invariant).
		if feeID := PlatformFeeUserID(); totalFee > 0 && feeID > 0 {
			me.updateBalanceRedis(ctx, feeID, quote, totalFee) //nolint:errcheck
			events = append(events, event{
				Topic: eventbus.TopicBalanceChanged,
				Payload: eventbus.BalanceEvent{
					UserID: feeID, Currency: quote,
					Delta: totalFee, Reason: "fee",
				},
			})
		}

		settledCount++

		if t.BuyOrder.Type == "LIMIT" {
			me.unlockBalanceRedis(ctx, t.BuyOrder.UserID, quote, total+buyerFee)
		}
		if t.SellOrder.Type == "LIMIT" {
			me.unlockBalanceRedis(ctx, t.SellOrder.UserID, base, t.Amount)
		}

		// BalanceEvents for projector DB sync
		for _, be := range []eventbus.BalanceEvent{
			{UserID: t.BuyOrder.UserID, Currency: quote, Delta: -(total + buyerFee), Reason: "trade"},
			{UserID: t.BuyOrder.UserID, Currency: base, Delta: t.Amount, Reason: "trade"},
			{UserID: t.SellOrder.UserID, Currency: base, Delta: -t.Amount, Reason: "trade"},
			{UserID: t.SellOrder.UserID, Currency: quote, Delta: total - sellerFee, Reason: "trade"},
		} {
			events = append(events, event{Topic: eventbus.TopicBalanceChanged, Payload: be})
		}

		events = append(events,
			event{
				Topic: eventbus.TopicTradeExecuted,
				Payload: eventbus.TradeEvent{
					Pair: order.Pair, BuyOrderID: t.BuyOrder.ID, SellOrderID: t.SellOrder.ID,
					BuyerID: t.BuyOrder.UserID, SellerID: t.SellOrder.UserID,
					Price: t.Price, Amount: t.Amount, Total: total,
					BuyerFee: buyerFee, SellerFee: sellerFee, Side: order.Side,
				},
			},
			event{
				Topic: eventbus.TopicOrderUpdated,
				Payload: eventbus.OrderEvent{
					OrderID: t.BuyOrder.ID, UserID: t.BuyOrder.UserID, Pair: order.Pair,
					Side: t.BuyOrder.Side, Type: t.BuyOrder.Type, Price: t.BuyOrder.Price,
					Amount: t.BuyOrder.Amount, FilledAmount: t.BuyOrder.FilledAmount,
					Status: t.BuyOrder.Status,
				},
			},
			event{
				Topic: eventbus.TopicOrderUpdated,
				Payload: eventbus.OrderEvent{
					OrderID: t.SellOrder.ID, UserID: t.SellOrder.UserID, Pair: order.Pair,
					Side: t.SellOrder.Side, Type: t.SellOrder.Type, Price: t.SellOrder.Price,
					Amount: t.SellOrder.Amount, FilledAmount: t.SellOrder.FilledAmount,
					Status: t.SellOrder.Status,
				},
			},
		)

		me.bus.PublishWS(ctx, "trades@"+order.Pair, map[string]interface{}{
			"price": t.Price, "amount": t.Amount, "side": order.Side,
		})
	}

	// Step 3: MARKET instant fill (demo mode — fill at realtime price).
	// Aggressor MARKET pays taker fee.
	if order.Type == "MARKET" && !order.IsFilled() {
		remaining := order.Remaining()
		marketPrice, _ := me.marketClient.GetPrice(ctx, order.Pair)
		if marketPrice > 0 && remaining > 0 {
			total := marketPrice * remaining
			_, takerRate := me.fees.Rates(order.UserID)
			fee := total * takerRate

			var balDeltas []eventbus.BalanceEvent
			if order.Side == "BUY" {
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

			// Credit demo-fill fee to fee wallet (single side here — counterparty
			// is synthetic in this demo path).
			if feeID := PlatformFeeUserID(); fee > 0 && feeID > 0 {
				me.updateBalanceRedis(ctx, feeID, quote, fee) //nolint:errcheck
				events = append(events, event{
					Topic: eventbus.TopicBalanceChanged,
					Payload: eventbus.BalanceEvent{
						UserID: feeID, Currency: quote, Delta: fee, Reason: "fee",
					},
				})
			}

			order.Price = marketPrice
			order.FilledAmount = order.Amount
			order.Status = "FILLED"

			buyID, sellID := order.ID, uint(0)
			buyerID, sellerID := order.UserID, uint(0)
			if order.Side == "SELL" {
				buyID, sellID = 0, order.ID
				buyerID, sellerID = 0, order.UserID
			}

			events = append(events, event{
				Topic: eventbus.TopicTradeExecuted,
				Payload: eventbus.TradeEvent{
					Pair: order.Pair, BuyOrderID: buyID, SellOrderID: sellID,
					BuyerID: buyerID, SellerID: sellerID,
					Price: marketPrice, Amount: remaining, Total: total,
					BuyerFee: fee, Side: order.Side,
				},
			})
			for _, be := range balDeltas {
				events = append(events, event{Topic: eventbus.TopicBalanceChanged, Payload: be})
			}

			me.bus.PublishWS(ctx, "trades@"+order.Pair, map[string]interface{}{
				"price": marketPrice, "amount": remaining, "side": order.Side,
			})
		}
	}

	// Add unfilled LIMIT to book
	if !order.IsFilled() && order.Type == "LIMIT" {
		book.AddOrder(order)
	}

	// Publish order status event
	events = append(events, event{
		Topic: eventbus.TopicOrderUpdated,
		Payload: eventbus.OrderEvent{
			OrderID: order.ID, UserID: order.UserID, Pair: order.Pair,
			Side: order.Side, Type: order.Type, Price: order.Price,
			Amount: order.Amount, FilledAmount: order.FilledAmount,
			Status: order.Status,
		},
	})

	for _, e := range events {
		me.bus.Publish(ctx, e.Topic, e.Payload)
	}

	me.bus.PublishWS(ctx, "depth@"+order.Pair, book.GetDepth(20))

	// Update orderbook size gauge (cheap — sums per-level lengths).
	metrics.OrderBookSize.WithLabelValues(order.Pair).Set(float64(bookOrderCount(book)))
	return nil
}

// bookOrderCount sums total resting orders across all price levels of a book.
// Cheap — used only by the metrics gauge update post-match.
func bookOrderCount(book *engine.OrderBook) int {
	d := book.GetDepth(1000)
	return len(d.Bids) + len(d.Asks)
}

// rollbackTrades reverts orderbook state for unsettled trades only.
func (me *MatchingEngine) rollbackTrades(order *model.Order, book *engine.OrderBook, trades []engine.TradeResult, settledCount int) {
	for i := settledCount; i < len(trades); i++ {
		t := trades[i]
		t.BuyOrder.FilledAmount -= t.Amount
		t.SellOrder.FilledAmount -= t.Amount
		if t.BuyOrder.FilledAmount <= 0 {
			t.BuyOrder.Status = "OPEN"
		} else {
			t.BuyOrder.Status = "PARTIAL"
		}
		if t.SellOrder.FilledAmount <= 0 {
			t.SellOrder.Status = "OPEN"
		} else {
			t.SellOrder.Status = "PARTIAL"
		}
		// Re-add the counterparty resting order ONLY if it was popped (fully filled previously).
		// Partial fills remain in the book — adding again would duplicate.
		if order.Side == "BUY" {
			if t.SellOrder.Status == "OPEN" {
				book.AddOrder(t.SellOrder)
			}
		} else {
			if t.BuyOrder.Status == "OPEN" {
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
		order.Status = "OPEN"
	} else {
		order.Status = "PARTIAL"
	}
}

// CancelOrder removes order from book and unlocks any LIMIT-locked balance.
// Defensive: callers should already have unlocked via OrderService.CancelOrder,
// but we tolerate either path.
func (me *MatchingEngine) CancelOrder(order *model.Order) {
	me.mu.RLock()
	book, ok := me.books[order.Pair]
	me.mu.RUnlock()
	if !ok {
		return
	}
	book.RemoveOrder(order.ID, order.Side)

	ctx := context.Background()
	me.bus.Publish(ctx, eventbus.TopicOrderUpdated, eventbus.OrderEvent{
		OrderID: order.ID, UserID: order.UserID, Pair: order.Pair,
		Side: order.Side, Type: order.Type, Price: order.Price,
		Amount: order.Amount, FilledAmount: order.FilledAmount,
		Status: "CANCELLED",
	})
	me.bus.PublishWS(ctx, "depth@"+order.Pair, book.GetDepth(20))
}

// GetCurrentPrice returns the current market price for a pair via gRPC.
func (me *MatchingEngine) GetCurrentPrice(pair string) float64 {
	price, _ := me.marketClient.GetPrice(context.Background(), pair)
	return price
}

// GetDepth returns order book depth (CQRS query — read from memory).
func (me *MatchingEngine) GetDepth(pair string, limit int) engine.DepthData {
	me.mu.RLock()
	book, ok := me.books[pair]
	me.mu.RUnlock()
	if !ok {
		return engine.DepthData{}
	}
	return book.GetDepth(limit)
}
