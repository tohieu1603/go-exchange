package domain

import (
	"sort"
	"sync"
)

// PriceLevel aggregates resting orders at one price.
type PriceLevel struct {
	Price  float64
	Orders []*Order
}

// OrderBook maintains price-time-priority bids and asks for one pair. It is the
// pure matching core — no persistence, balances, or events.
type OrderBook struct {
	Pair string
	Bids []PriceLevel // price DESC
	Asks []PriceLevel // price ASC
	mu   sync.RWMutex
}

func NewOrderBook(pair string) *OrderBook { return &OrderBook{Pair: pair} }

// AddOrder inserts a resting (unmatched) order.
func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	if order.Side == SideBuy {
		ob.addToBids(order)
	} else {
		ob.addToAsks(order)
	}
}

func (ob *OrderBook) addToBids(order *Order) {
	for i, level := range ob.Bids {
		if level.Price == order.Price {
			ob.Bids[i].Orders = append(ob.Bids[i].Orders, order)
			return
		}
	}
	ob.Bids = append(ob.Bids, PriceLevel{Price: order.Price, Orders: []*Order{order}})
	sort.Slice(ob.Bids, func(i, j int) bool { return ob.Bids[i].Price > ob.Bids[j].Price })
}

func (ob *OrderBook) addToAsks(order *Order) {
	for i, level := range ob.Asks {
		if level.Price == order.Price {
			ob.Asks[i].Orders = append(ob.Asks[i].Orders, order)
			return
		}
	}
	ob.Asks = append(ob.Asks, PriceLevel{Price: order.Price, Orders: []*Order{order}})
	sort.Slice(ob.Asks, func(i, j int) bool { return ob.Asks[i].Price < ob.Asks[j].Price })
}

// RemoveOrder removes a resting order (cancel).
func (ob *OrderBook) RemoveOrder(orderID uint, side string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	levels := &ob.Bids
	if side == SideSell {
		levels = &ob.Asks
	}
	for i, level := range *levels {
		for j, o := range level.Orders {
			if o.ID == orderID {
				(*levels)[i].Orders = append(level.Orders[:j], level.Orders[j+1:]...)
				if len((*levels)[i].Orders) == 0 {
					*levels = append((*levels)[:i], (*levels)[i+1:]...)
				}
				return
			}
		}
	}
}

// Match matches an incoming order against the book, returning the trades made.
func (ob *OrderBook) Match(order *Order) []TradeResult {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	if order.Side == SideBuy {
		return ob.matchBuy(order)
	}
	return ob.matchSell(order)
}

func (ob *OrderBook) matchBuy(buyOrder *Order) []TradeResult {
	var trades []TradeResult
	remaining := buyOrder.Remaining()
	for remaining > 0 && len(ob.Asks) > 0 {
		bestAsk := &ob.Asks[0]
		if buyOrder.Type == TypeLimit && bestAsk.Price > buyOrder.Price {
			break
		}
		for len(bestAsk.Orders) > 0 && remaining > 0 {
			sellOrder := bestAsk.Orders[0]
			matchQty := minFloat(remaining, sellOrder.Remaining())
			trades = append(trades, TradeResult{BuyOrder: buyOrder, SellOrder: sellOrder, Price: bestAsk.Price, Amount: matchQty})
			remaining -= matchQty
			buyOrder.FilledAmount += matchQty
			sellOrder.FilledAmount += matchQty
			if sellOrder.IsFilled() {
				sellOrder.Status = StatusFilled
				bestAsk.Orders = bestAsk.Orders[1:]
			} else {
				sellOrder.Status = StatusPartial
			}
		}
		if len(bestAsk.Orders) == 0 {
			ob.Asks = ob.Asks[1:]
		}
	}
	finalizeTaker(buyOrder)
	return trades
}

func (ob *OrderBook) matchSell(sellOrder *Order) []TradeResult {
	var trades []TradeResult
	remaining := sellOrder.Remaining()
	for remaining > 0 && len(ob.Bids) > 0 {
		bestBid := &ob.Bids[0]
		if sellOrder.Type == TypeLimit && bestBid.Price < sellOrder.Price {
			break
		}
		for len(bestBid.Orders) > 0 && remaining > 0 {
			buyOrder := bestBid.Orders[0]
			matchQty := minFloat(remaining, buyOrder.Remaining())
			trades = append(trades, TradeResult{BuyOrder: buyOrder, SellOrder: sellOrder, Price: bestBid.Price, Amount: matchQty})
			remaining -= matchQty
			sellOrder.FilledAmount += matchQty
			buyOrder.FilledAmount += matchQty
			if buyOrder.IsFilled() {
				buyOrder.Status = StatusFilled
				bestBid.Orders = bestBid.Orders[1:]
			} else {
				buyOrder.Status = StatusPartial
			}
		}
		if len(bestBid.Orders) == 0 {
			ob.Bids = ob.Bids[1:]
		}
	}
	finalizeTaker(sellOrder)
	return trades
}

func finalizeTaker(o *Order) {
	if o.IsFilled() {
		o.Status = StatusFilled
	} else if o.FilledAmount > 0 {
		o.Status = StatusPartial
	}
}

// GetDepth returns aggregated depth for streaming.
func (ob *OrderBook) GetDepth(limit int) DepthData {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	bids := make([][2]float64, 0, limit)
	asks := make([][2]float64, 0, limit)
	for i, level := range ob.Bids {
		if i >= limit {
			break
		}
		var qty float64
		for _, o := range level.Orders {
			qty += o.Remaining()
		}
		bids = append(bids, [2]float64{level.Price, qty})
	}
	for i, level := range ob.Asks {
		if i >= limit {
			break
		}
		var qty float64
		for _, o := range level.Orders {
			qty += o.Remaining()
		}
		asks = append(asks, [2]float64{level.Price, qty})
	}
	return DepthData{Bids: bids, Asks: asks}
}

// TradeResult is a match produced by the book.
type TradeResult struct {
	BuyOrder  *Order
	SellOrder *Order
	Price     float64
	Amount    float64
}

// DepthData is an aggregated order-book snapshot for streaming.
type DepthData struct {
	Bids [][2]float64 `json:"bids"`
	Asks [][2]float64 `json:"asks"`
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
