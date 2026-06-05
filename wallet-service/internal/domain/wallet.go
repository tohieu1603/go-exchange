package domain

// Wallet is a user's balance in a single currency.
//
// Balance arithmetic (credit/debit/lock/unlock) is performed *atomically* at the
// persistence layer via conditional SQL UPDATEs and at the Redis hot path via
// Lua scripts — never by loading the row, mutating in memory, and writing back.
// That is a deliberate concurrency choice for an exchange: it removes the
// read-modify-write race entirely. The aggregate therefore exposes read helpers
// and invariant checks; the mutating operations are expressed as repository port
// methods (see WalletRepository).
type Wallet struct {
	UserID   uint
	Currency string
	Balance  float64
	Locked   float64
}

// Available is the spendable balance (total minus the amount reserved by open
// orders / pending withdrawals).
func (w Wallet) Available() float64 { return w.Balance - w.Locked }

// CanDebit reports whether amount can be withdrawn from the available balance.
func (w Wallet) CanDebit(amount float64) bool { return amount > 0 && w.Available() >= amount }
