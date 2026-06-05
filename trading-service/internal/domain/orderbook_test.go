package domain

import "testing"

func limitSell(id uint, price, amount float64) *Order {
	return &Order{ID: id, UserID: id + 100, Pair: "BTC_USDT", Side: SideSell, Type: TypeLimit, Price: price, Amount: amount, Status: StatusOpen}
}

func TestOrderBook_LimitMatchFull(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")
	ask := limitSell(1, 100, 1)
	ob.AddOrder(ask)

	buy := &Order{ID: 2, UserID: 9, Pair: "BTC_USDT", Side: SideBuy, Type: TypeLimit, Price: 100, Amount: 1, Status: StatusOpen}
	trades := ob.Match(buy)
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Amount != 1 {
		t.Fatalf("trade = %+v, want price 100 amount 1", trades[0])
	}
	if !buy.IsFilled() || buy.Status != StatusFilled {
		t.Fatalf("buy should be FILLED, got %s", buy.Status)
	}
	if ask.Status != StatusFilled {
		t.Fatalf("ask should be FILLED, got %s", ask.Status)
	}
}

func TestOrderBook_LimitNoCross(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")
	ob.AddOrder(limitSell(1, 100, 1))
	buy := &Order{ID: 2, Side: SideBuy, Type: TypeLimit, Pair: "BTC_USDT", Price: 99, Amount: 1, Status: StatusOpen}
	if trades := ob.Match(buy); len(trades) != 0 {
		t.Fatalf("a buy below the best ask must not cross, got %d trades", len(trades))
	}
	if buy.FilledAmount != 0 {
		t.Fatalf("buy must be unfilled, filled=%v", buy.FilledAmount)
	}
}

func TestOrderBook_PartialFill(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")
	ob.AddOrder(limitSell(1, 100, 1))
	buy := &Order{ID: 2, Side: SideBuy, Type: TypeLimit, Pair: "BTC_USDT", Price: 100, Amount: 2, Status: StatusOpen}
	trades := ob.Match(buy)
	if len(trades) != 1 || trades[0].Amount != 1 {
		t.Fatalf("expected one 1-unit trade, got %+v", trades)
	}
	if buy.Status != StatusPartial || buy.Remaining() != 1 {
		t.Fatalf("buy should be PARTIAL with 1 remaining, got status=%s remaining=%v", buy.Status, buy.Remaining())
	}
}

func TestOrderBook_PricePriority(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")
	ob.AddOrder(limitSell(1, 101, 1)) // worse price first
	ob.AddOrder(limitSell(2, 100, 1)) // better price
	buy := &Order{ID: 3, Side: SideBuy, Type: TypeLimit, Pair: "BTC_USDT", Price: 101, Amount: 1, Status: StatusOpen}
	trades := ob.Match(buy)
	if len(trades) != 1 || trades[0].Price != 100 {
		t.Fatalf("best (lowest) ask must fill first at 100, got %+v", trades)
	}
}

func TestOrderBook_MarketSweepsLevels(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")
	ob.AddOrder(limitSell(1, 100, 1))
	ob.AddOrder(limitSell(2, 101, 1))
	buy := &Order{ID: 3, Side: SideBuy, Type: TypeMarket, Pair: "BTC_USDT", Amount: 2, Status: StatusOpen}
	trades := ob.Match(buy)
	if len(trades) != 2 {
		t.Fatalf("market buy should sweep both levels, got %d trades", len(trades))
	}
	if !buy.IsFilled() {
		t.Fatal("market buy should be fully filled")
	}
}
