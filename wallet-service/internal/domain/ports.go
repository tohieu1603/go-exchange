package domain

import "context"

// Repository ports — owned by the domain, implemented by infrastructure adapters.
// All methods take a context.Context (which also carries the active transaction,
// injected by the application's TxManager) and speak only in domain types — no
// *gorm.DB leaks across this boundary.

// WalletRepository persists wallets. The balance-mutating methods are atomic
// (conditional SQL UPDATE) by design — see the Wallet doc comment.
type WalletRepository interface {
	FindByUserAndCurrency(ctx context.Context, userID uint, currency string) (*Wallet, error)
	FindAllByUser(ctx context.Context, userID uint) ([]Wallet, error)
	// ListAll streams every wallet — used only for cold-start cache warming.
	ListAll(ctx context.Context) ([]Wallet, error)
	// UpdateBalance adds delta (signed) to balance. When delta < 0 the UPDATE is
	// guarded so the balance cannot go negative (returns no rows → caller treats
	// as insufficient).
	UpdateBalance(ctx context.Context, userID uint, currency string, delta float64) error
	// Lock moves amount from available into locked; fails if available < amount.
	Lock(ctx context.Context, userID uint, currency string, amount float64) error
	// Unlock releases amount from locked (floored at zero).
	Unlock(ctx context.Context, userID uint, currency string, amount float64) error
	// IncreaseLocked adds amount to locked unconditionally — used by the
	// balance.changed projector to mirror a lock that already succeeded on the
	// Redis hot path (the availability check happened there, not here).
	IncreaseLocked(ctx context.Context, userID uint, currency string, amount float64) error
	CreateBatch(ctx context.Context, wallets []Wallet) error
	// Upsert credits balance, inserting the wallet row if it does not yet exist.
	Upsert(ctx context.Context, userID uint, currency string, balance float64) error
	CountByUser(ctx context.Context, userID uint) (int64, error)
}

// DepositRepository persists deposits.
type DepositRepository interface {
	Create(ctx context.Context, d *Deposit) error
	FindByID(ctx context.Context, id uint) (*Deposit, error)
	FindByOrderCode(ctx context.Context, code string) (*Deposit, error)
	FindByUser(ctx context.Context, userID uint, page, size int) ([]Deposit, int64, error)
	Save(ctx context.Context, d *Deposit) error
	// ListAdmin is a read model: paginated deposits filtered by status and a
	// free-text search on the owning user's email.
	ListAdmin(ctx context.Context, page, size int, search, status string) ([]Deposit, int64, error)
}

// WithdrawalRepository persists withdrawals.
type WithdrawalRepository interface {
	Create(ctx context.Context, w *Withdrawal) error
	FindByID(ctx context.Context, id uint) (*Withdrawal, error)
	FindByUser(ctx context.Context, userID uint, page, size int) ([]Withdrawal, int64, error)
	Save(ctx context.Context, w *Withdrawal) error
	FindLatestPendingByUser(ctx context.Context, userID uint) (*Withdrawal, error)
	ListAdmin(ctx context.Context, page, size int, search, status string) ([]Withdrawal, int64, error)
}
