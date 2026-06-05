package domain

import "testing"

func TestWallet_AvailableAndCanDebit(t *testing.T) {
	w := Wallet{UserID: 1, Currency: "USDT", Balance: 100, Locked: 30}
	if got := w.Available(); got != 70 {
		t.Fatalf("Available = %v, want 70", got)
	}
	if !w.CanDebit(70) {
		t.Fatal("should allow debiting exactly the available balance")
	}
	if w.CanDebit(70.01) {
		t.Fatal("must not allow debiting more than available")
	}
	if w.CanDebit(0) || w.CanDebit(-5) {
		t.Fatal("non-positive debits are invalid")
	}
}

func TestNewCurrency(t *testing.T) {
	for _, ok := range []string{"usdt", " BTC ", "VND", "USD1"} {
		if _, err := NewCurrency(ok); err != nil {
			t.Fatalf("NewCurrency(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "  ", "US DT", "BTC/USDT", "TOOLONGSYMBOL"} {
		if _, err := NewCurrency(bad); err == nil {
			t.Fatalf("NewCurrency(%q) should have failed", bad)
		}
	}
	if c, _ := NewCurrency(" btc "); c.String() != "BTC" {
		t.Fatalf("currency should normalise to BTC, got %q", c)
	}
}
