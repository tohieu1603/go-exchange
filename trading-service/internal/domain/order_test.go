package domain

import "testing"

func TestOrder_RemainingFilledCancellable(t *testing.T) {
	o := &Order{Amount: 5, FilledAmount: 2, Status: StatusPartial}
	if o.Remaining() != 3 {
		t.Fatalf("remaining = %v, want 3", o.Remaining())
	}
	if o.IsFilled() {
		t.Fatal("partial order is not filled")
	}
	if !o.IsCancellable() {
		t.Fatal("PARTIAL order is cancellable")
	}
	o.FilledAmount = 5
	if !o.IsFilled() {
		t.Fatal("fully-filled order")
	}
	o.Status = StatusFilled
	if o.IsCancellable() {
		t.Fatal("FILLED order must not be cancellable")
	}
}
