package domain

import (
	"errors"
	"testing"
)

func TestNewWithdrawal_Validation(t *testing.T) {
	if _, err := NewWithdrawal(1, 0, "VCB", "123", "ALICE"); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("zero amount must error, got %v", err)
	}
	if _, err := NewWithdrawal(1, 100, "", "123", "ALICE"); err == nil {
		t.Fatal("missing bank code must error")
	}
}

func TestWithdrawal_ApproveAndRejectAreTerminal(t *testing.T) {
	w, err := NewWithdrawal(1, 500000, "VCB", "123", "ALICE")
	if err != nil {
		t.Fatalf("NewWithdrawal: %v", err)
	}
	if w.Status != WithdrawalPending {
		t.Fatalf("new withdrawal must be PENDING, got %s", w.Status)
	}
	if err := w.Approve(); err != nil {
		t.Fatalf("approve pending should succeed: %v", err)
	}
	if err := w.Reject(); !errors.Is(err, ErrWithdrawalNotPending) {
		t.Fatalf("reject after approve must error, got %v", err)
	}
}

func TestWithdrawal_Reject(t *testing.T) {
	w, _ := NewWithdrawal(1, 500000, "VCB", "123", "ALICE")
	if err := w.Reject(); err != nil {
		t.Fatalf("reject pending should succeed: %v", err)
	}
	if w.Status != WithdrawalRejected {
		t.Fatalf("status must be REJECTED, got %s", w.Status)
	}
	if err := w.Approve(); !errors.Is(err, ErrWithdrawalNotPending) {
		t.Fatalf("approve after reject must error, got %v", err)
	}
}
