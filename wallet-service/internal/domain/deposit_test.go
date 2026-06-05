package domain

import (
	"errors"
	"testing"
)

func TestNewDeposit_RejectsNonPositiveAmount(t *testing.T) {
	if _, err := NewDeposit(1, 0, 0, 25000, "DEP-1", ""); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("want ErrInvalidAmount for zero amount, got %v", err)
	}
}

func TestDeposit_ConfirmLifecycle(t *testing.T) {
	d, err := NewDeposit(1, 250000, 10, 25000, "DEP-1", "qr")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	if d.Status != DepositPending {
		t.Fatalf("new deposit must be PENDING, got %s", d.Status)
	}
	if err := d.Confirm(); err != nil {
		t.Fatalf("first Confirm should succeed: %v", err)
	}
	if d.Status != DepositConfirmed {
		t.Fatalf("status must be CONFIRMED, got %s", d.Status)
	}
	// Idempotency guard: confirming again (or failing) is rejected.
	if err := d.Confirm(); !errors.Is(err, ErrDepositNotPending) {
		t.Fatalf("double-confirm must error, got %v", err)
	}
	if err := d.Fail(); !errors.Is(err, ErrDepositNotPending) {
		t.Fatalf("fail after confirm must error, got %v", err)
	}
}

func TestDeposit_Fail(t *testing.T) {
	d, _ := NewDeposit(1, 250000, 10, 25000, "DEP-2", "")
	if err := d.Fail(); err != nil {
		t.Fatalf("Fail on pending should succeed: %v", err)
	}
	if d.Status != DepositFailed {
		t.Fatalf("status must be FAILED, got %s", d.Status)
	}
}
