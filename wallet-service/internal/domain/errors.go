// Package domain holds the wallet bounded context's enterprise rules: the
// Wallet / Deposit / Withdrawal entities, their invariants and state machines,
// value objects, typed errors, and the repository ports. It depends on nothing
// outside the standard library — no gorm, gin, redis, or gRPC.
package domain

import "errors"

// Typed domain errors. The interfaces layer maps these to HTTP/gRPC statuses;
// callers compare with errors.Is rather than matching strings.
var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrAccountLocked       = errors.New("account is locked")
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrInvalidAmount       = errors.New("amount must be positive")
	ErrInvalidCurrency     = errors.New("invalid currency")

	ErrDepositNotFound   = errors.New("deposit not found")
	ErrDepositNotPending = errors.New("deposit is not in PENDING status")

	ErrWithdrawalNotFound   = errors.New("withdrawal not found")
	ErrWithdrawalNotPending = errors.New("withdrawal is not in PENDING status")

	// ErrAmountOutOfRange is returned when an amount violates the platform's
	// configured min/max for the operation; wrap with context for the message.
	ErrAmountOutOfRange = errors.New("amount out of allowed range")
)
