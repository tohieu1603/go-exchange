package engine

import (
	"sort"
	"sync"

	"github.com/cryptox/trading-service/internal/model"
)

// PriceLevel represents aggregated orders at one price
type PriceLevel struct {
	Price  float64
	Orders []*model.Order
}

// OrderBook maintains sorted bids and asks for a trading pair
type OrderBook struct {
	Pair string
	Bids []PriceLevel // sorted by price DESC (highest first)
	Asks []PriceLevel // sorted by price ASC (lowest first)
	mu   sync.RWMutex
}

func NewOrderBook(pair string) *OrderBook {
	return &OrderBook{Pair: pair}
}

// AddOrder inserts an order into the book (for LIMIT orders that aren't fully matched)
func (ob *OrderBook) AddOrder(order *model.Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if order.Side == "BUY" {
		ob.addToBids(order)
	} else {
		ob.addToAsks(order)
	}
}

func (ob *OrderBook) addToBids(order *model.Order) {
	for i, level := range ob.Bids {
		if level.Price == order.Price {
			ob.Bids[i].Orders = append(ob.Bids[i].Orders, order)
			return
		}
	}
	ob.Bids = append(ob.Bids, PriceLevel{Price: order.Price, Orders: []*model.Order{order}})
	sort.Slice(ob.Bids, func(i, j int) bool { return ob.Bids[i].Price > ob.Bids[j].Price })
}

func (ob *OrderBook) addToAsks(order *model.Order) {
	for i, level := range ob.Asks {
		if level.Price == order.Price {
			ob.Asks[i].Orders = append(ob.Asks[i].Orders, order)
			return
		}
	}
	ob.Asks = append(ob.Asks, PriceLevel{Price: order.Price, Orders: []*model.Order{order}})
	sort.Slice(ob.Asks, func(i, j int) bool { return ob.Asks[i].Price < ob.Asks[j].Price })
}

// RemoveOrder removes a specific order (for cancel)
func (ob *OrderBook) RemoveOrder(orderID uint, side string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	levels := &ob.Bids
	if side == "SELL" {
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

// Match attempts to match an incoming order against the book.
// Returns list of trades generated.
func (ob *OrderBook) Match(order *model.Order) []TradeResult {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if order.Side == "BUY" {
		return ob.matchBuy(order)
	}
	return ob.matchSell(order)
}

func (ob *OrderBook) matchBuy(buyOrder *model.Order) []TradeResult {
	var trades []TradeResult
	remaining := buyOrder.Remaining()

	for remaining > 0 && len(ob.Asks) > 0 {
		bestAsk := &ob.Asks[0]
		if buyOrder.Type == "LIMIT" && bestAsk.Price > buyOrder.Price {
			break
		}

		for len(bestAsk.Orders) > 0 && remaining > 0 {
			sellOrder := bestAsk.Orders[0]
			sellRemaining := sellOrder.Remaining()

			matchQty := minFloat(remaining, sellRemaining)
			matchPrice := bestAsk.Price

			trades = append(trades, TradeResult{
				BuyOrder:  buyOrder,
				SellOrder: sellOrder,
				Price:     matchPrice,
				Amount:    matchQty,
			})

			remaining -= matchQty
			buyOrder.FilledAmount += matchQty
			sellOrder.FilledAmount += matchQty

			if sellOrder.IsFilled() {
				sellOrder.Status = "FILLED"
				bestAsk.Orders = bestAsk.Orders[1:]
			} else {
				sellOrder.Status = "PARTIAL"
			}
		}

		if len(bestAsk.Orders) == 0 {
			ob.Asks = ob.Asks[1:]
		}
	}

	if buyOrder.IsFilled() {
		buyOrder.Status = "FILLED"
	} else if buyOrder.FilledAmount > 0 {
		buyOrder.Status = "PARTIAL"
	}
	return trades
}

func (ob *OrderBook) matchSell(sellOrder *model.Order) []TradeResult {
	var trades []TradeResult
	remaining := sellOrder.Remaining()

	for remaining > 0 && len(ob.Bids) > 0 {
		bestBid := &ob.Bids[0]
		if sellOrder.Type == "LIMIT" && bestBid.Price < sellOrder.Price {
			break
		}

		for len(bestBid.Orders) > 0 && remaining > 0 {
			buyOrder := bestBid.Orders[0]
			buyRemaining := buyOrder.Remaining()

			matchQty := minFloat(remaining, buyRemaining)
			matchPrice := bestBid.Price

			trades = append(trades, TradeResult{
				BuyOrder:  buyOrder,
				SellOrder: sellOrder,
				Price:     matchPrice,
				Amount:    matchQty,
			})

			remaining -= matchQty
			sellOrder.FilledAmount += matchQty
			buyOrder.FilledAmount += matchQty

			if buyOrder.IsFilled() {
				buyOrder.Status = "FILLED"
				bestBid.Orders = bestBid.Orders[1:]
			} else {
				buyOrder.Status = "PARTIAL"
			}
		}

		if len(bestBid.Orders) == 0 {
			ob.Bids = ob.Bids[1:]
		}
	}

	if sellOrder.IsFilled() {
		sellOrder.Status = "FILLED"
	} else if sellOrder.FilledAmount > 0 {
		sellOrder.Status = "PARTIAL"
	}
	return trades
}

// GetDepth returns aggregated order book depth for WebSocket
func (ob *OrderBook) GetDepth(limit int) DepthData {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids := make([][2]float64, 0, limit)
	asks := make([][2]float64, 0, limit)

	for i, level := range ob.Bids {
		if i >= limit {
			break
		}
		var totalQty float64
		for _, o := range level.Orders {
			totalQty += o.Remaining()
		}
		bids = append(bids, [2]float64{level.Price, totalQty})
	}

	for i, level := range ob.Asks {
		if i >= limit {
			break
		}
		var totalQty float64
		for _, o := range level.Orders {
			totalQty += o.Remaining()
		}
		asks = append(asks, [2]float64{level.Price, totalQty})
	}

	return DepthData{Bids: bids, Asks: asks}
}

// TradeResult from matching
type TradeResult struct {
	BuyOrder  *model.Order
	SellOrder *model.Order
	Price     float64
	Amount    float64
}

// DepthData for WebSocket
type DepthData struct {
	Bids [][2]float64 `json:"bids"` // [price, quantity]
	Asks [][2]float64 `json:"asks"`
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
