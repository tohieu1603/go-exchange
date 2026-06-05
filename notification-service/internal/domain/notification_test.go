package domain

import "testing"

func TestNewNotification_StartsUnread(t *testing.T) {
	n := NewNotification(7, "DEPOSIT_CONFIRMED", "Deposit", "ok", "BTC_USDT")
	if n.IsRead {
		t.Fatal("a new notification must be unread")
	}
	if n.UserID != 7 || n.Type != "DEPOSIT_CONFIRMED" {
		t.Fatalf("unexpected fields: %+v", n)
	}
	n.MarkRead()
	if !n.IsRead {
		t.Fatal("MarkRead must set IsRead=true")
	}
}
