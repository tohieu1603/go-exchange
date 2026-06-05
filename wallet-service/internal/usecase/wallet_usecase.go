package usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/shared/utils"
	"github.com/cryptox/wallet-service/internal/domain"
)

const usdt = "USDT"

// SepayConfig is the merchant bank account a deposit QR points at.
type SepayConfig struct {
	BankCode    string
	BankAccount string
	AccountName string
}

// WalletUseCase is the wallet bounded context's application service. It depends
// only on domain repository ports and the outbound ports in this package.
type WalletUseCase struct {
	wallets     domain.WalletRepository
	deposits    domain.DepositRepository
	withdrawals domain.WithdrawalRepository
	tx          TxManager
	cache       BalanceCache
	settings    SettingsGate
	rate        RateProvider
	bus         EventPublisher
	sepay       SepayConfig
}

func NewWalletUseCase(
	wallets domain.WalletRepository,
	deposits domain.DepositRepository,
	withdrawals domain.WithdrawalRepository,
	tx TxManager,
	cache BalanceCache,
	settings SettingsGate,
	rate RateProvider,
	bus EventPublisher,
	sepay SepayConfig,
) *WalletUseCase {
	return &WalletUseCase{
		wallets: wallets, deposits: deposits, withdrawals: withdrawals,
		tx: tx, cache: cache, settings: settings, rate: rate, bus: bus, sepay: sepay,
	}
}

// ── balance reads ────────────────────────────────────────────────────────────

func (s *WalletUseCase) GetBalances(ctx context.Context, userID uint) ([]domain.Wallet, error) {
	return s.wallets.FindAllByUser(ctx, userID)
}

// ensureCached lazily seeds the Redis cache from the DB on first touch.
func (s *WalletUseCase) ensureCached(ctx context.Context, userID uint, currency string) {
	if _, ok := s.cache.GetBalance(ctx, userID, currency); ok {
		return
	}
	w, err := s.wallets.FindByUserAndCurrency(ctx, userID, currency)
	if err != nil {
		return
	}
	s.cache.LoadFromDB(ctx, userID, currency, w.Balance, w.Locked)
}

// WarmCache seeds every wallet into Redis at cold-start without clobbering live state.
func (s *WalletUseCase) WarmCache(ctx context.Context) (int, error) {
	rows, err := s.wallets.ListAll(ctx)
	if err != nil {
		return 0, err
	}
	for _, w := range rows {
		s.cache.WarmIfMissing(ctx, w.UserID, w.Currency, w.Balance, w.Locked)
	}
	return len(rows), nil
}

// ── deposits ─────────────────────────────────────────────────────────────────

// CreateDeposit opens a PENDING VND deposit and returns it with a payment QR.
func (s *WalletUseCase) CreateDeposit(ctx context.Context, userID uint, amountVND float64) (*domain.Deposit, error) {
	if s.settings.AccountLocked(ctx, userID) {
		return nil, domain.ErrAccountLocked
	}
	ps := s.settings.PlatformSettings(ctx)
	if ps.MinDeposit > 0 && amountVND < ps.MinDeposit {
		return nil, fmt.Errorf("%w: minimum deposit is %.0f VND", domain.ErrAmountOutOfRange, ps.MinDeposit)
	}
	if ps.MaxDeposit > 0 && amountVND > ps.MaxDeposit {
		return nil, fmt.Errorf("%w: maximum deposit is %.0f VND", domain.ErrAmountOutOfRange, ps.MaxDeposit)
	}

	rate := s.rate.VNDRate()
	amountUSDT := amountVND / rate
	if ps.DepositFeePercent > 0 {
		amountUSDT *= 1 - ps.DepositFeePercent/100
	}

	orderCode := fmt.Sprintf("DEP-%d-%d", userID, time.Now().UnixMilli())
	qrURL := utils.GenerateSepayQR(s.sepay.BankAccount, s.sepay.BankCode, amountVND, orderCode)

	deposit, err := domain.NewDeposit(userID, amountVND, amountUSDT, rate, orderCode, qrURL)
	if err != nil {
		return nil, err
	}
	if err := s.deposits.Create(ctx, deposit); err != nil {
		return nil, err
	}
	return deposit, nil
}

// ConfirmDeposit credits USDT once the bank webhook (or admin) confirms receipt.
// The wallet credit and the deposit state change commit atomically; the Redis
// cache is credited only AFTER commit so a rollback cannot leave a ghost balance.
func (s *WalletUseCase) ConfirmDeposit(ctx context.Context, orderCode string) error {
	deposit, err := s.deposits.FindByOrderCode(ctx, orderCode)
	if err != nil {
		return domain.ErrDepositNotFound
	}
	if err := deposit.Confirm(); err != nil {
		return err
	}
	if err := s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.wallets.Upsert(ctx, deposit.UserID, usdt, deposit.AmountUSDT); err != nil {
			return err
		}
		return s.deposits.Save(ctx, deposit)
	}); err != nil {
		return err
	}

	s.cache.Credit(ctx, deposit.UserID, usdt, deposit.AmountUSDT)
	s.publishNotification(ctx, deposit.UserID, "DEPOSIT_CONFIRMED", "Deposit Confirmed",
		fmt.Sprintf("$%.2f USDT deposited", deposit.AmountUSDT))
	return nil
}

func (s *WalletUseCase) GetDepositHistory(ctx context.Context, userID uint, page, size int) ([]domain.Deposit, int64, error) {
	return s.deposits.FindByUser(ctx, userID, page, size)
}

// ── withdrawals ──────────────────────────────────────────────────────────────

// CreateWithdrawal reserves the user's USDT (cache + DB) and records a PENDING
// payout. If the DB transaction fails the Redis reservation is compensated.
func (s *WalletUseCase) CreateWithdrawal(ctx context.Context, userID uint, amountUSDT float64, bankCode, bankAccount, accountName string) (*domain.Withdrawal, error) {
	if s.settings.AccountLocked(ctx, userID) {
		return nil, domain.ErrAccountLocked
	}
	rate := s.rate.VNDRate()
	vndAmount := amountUSDT * rate

	ps := s.settings.PlatformSettings(ctx)
	if ps.MinWithdraw > 0 && vndAmount < ps.MinWithdraw {
		return nil, fmt.Errorf("%w: minimum withdrawal is %.0f VND", domain.ErrAmountOutOfRange, ps.MinWithdraw)
	}
	if ps.MaxWithdraw > 0 && vndAmount > ps.MaxWithdraw {
		return nil, fmt.Errorf("%w: maximum withdrawal is %.0f VND", domain.ErrAmountOutOfRange, ps.MaxWithdraw)
	}

	// Bonus-locked funds are not withdrawable: withdrawable = available - bonusRemaining.
	if bonus := s.settings.BonusRemaining(ctx, userID); bonus > 0 {
		s.ensureCached(ctx, userID, usdt)
		total, _ := s.cache.GetBalance(ctx, userID, usdt)
		locked := s.cache.GetLocked(ctx, userID, usdt)
		withdrawable := total - locked - bonus
		if withdrawable < 0 {
			withdrawable = 0
		}
		if amountUSDT > withdrawable {
			return nil, fmt.Errorf("%w: limited to %.4f USDT (balance minus active bonus)", domain.ErrAmountOutOfRange, withdrawable)
		}
	}

	withdrawal, err := domain.NewWithdrawal(userID, vndAmount, bankCode, bankAccount, accountName)
	if err != nil {
		return nil, err
	}

	// Reserve on the Redis hot path first (authoritative availability check).
	s.ensureCached(ctx, userID, usdt)
	if err := s.cache.Lock(ctx, userID, usdt, amountUSDT); err != nil {
		return nil, domain.ErrInsufficientBalance
	}
	if err := s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.wallets.Lock(ctx, userID, usdt, amountUSDT); err != nil {
			return domain.ErrInsufficientBalance
		}
		return s.withdrawals.Create(ctx, withdrawal)
	}); err != nil {
		s.cache.Unlock(ctx, userID, usdt, amountUSDT) // compensate the Redis reservation
		return nil, err
	}
	return withdrawal, nil
}

func (s *WalletUseCase) GetWithdrawalHistory(ctx context.Context, userID uint, page, size int) ([]domain.Withdrawal, int64, error) {
	return s.withdrawals.FindByUser(ctx, userID, page, size)
}

// ── Redis hot-path balance ops (gRPC-facing) ─────────────────────────────────

// AdjustBalance applies a signed delta on the Redis hot path and publishes a
// balance.changed event so the DB projector + audit stay in sync.
func (s *WalletUseCase) AdjustBalance(ctx context.Context, userID uint, currency string, delta float64) error {
	if delta == 0 {
		return nil
	}
	s.ensureCached(ctx, userID, currency)
	if delta < 0 {
		if _, err := s.cache.Deduct(ctx, userID, currency, -delta); err != nil {
			return domain.ErrInsufficientBalance
		}
	} else {
		s.cache.Credit(ctx, userID, currency, delta)
	}
	s.publishBalance(ctx, userID, currency, delta, "grpc", "")
	return nil
}

func (s *WalletUseCase) LockBalance(ctx context.Context, userID uint, currency string, amount float64) error {
	s.ensureCached(ctx, userID, currency)
	if err := s.cache.Lock(ctx, userID, currency, amount); err != nil {
		return domain.ErrInsufficientBalance
	}
	s.publishBalance(ctx, userID, currency, amount, "lock", "")
	return nil
}

func (s *WalletUseCase) UnlockBalance(ctx context.Context, userID uint, currency string, amount float64) error {
	s.cache.Unlock(ctx, userID, currency, amount)
	s.publishBalance(ctx, userID, currency, amount, "unlock", "")
	return nil
}

func (s *WalletUseCase) CheckBalance(ctx context.Context, userID uint, currency string, needed float64) error {
	s.ensureCached(ctx, userID, currency)
	bal, ok := s.cache.GetBalance(ctx, userID, currency)
	if !ok {
		return domain.ErrWalletNotFound
	}
	if bal-s.cache.GetLocked(ctx, userID, currency) < needed {
		return domain.ErrInsufficientBalance
	}
	return nil
}

func (s *WalletUseCase) GetBalance(ctx context.Context, userID uint, currency string) (balance, locked float64) {
	s.ensureCached(ctx, userID, currency)
	bal, _ := s.cache.GetBalance(ctx, userID, currency)
	return bal, s.cache.GetLocked(ctx, userID, currency)
}

// ── admin ────────────────────────────────────────────────────────────────────

func (s *WalletUseCase) AdminListDeposits(ctx context.Context, page, size int, search, status string) ([]domain.Deposit, int64, error) {
	return s.deposits.ListAdmin(ctx, page, size, search, status)
}

func (s *WalletUseCase) AdminRejectDeposit(ctx context.Context, depositID uint) error {
	d, err := s.deposits.FindByID(ctx, depositID)
	if err != nil {
		return domain.ErrDepositNotFound
	}
	if err := d.Fail(); err != nil {
		return err
	}
	return s.deposits.Save(ctx, d)
}

func (s *WalletUseCase) AdminListWithdrawals(ctx context.Context, page, size int, search, status string) ([]domain.Withdrawal, int64, error) {
	return s.withdrawals.ListAdmin(ctx, page, size, search, status)
}

func (s *WalletUseCase) AdminApproveWithdrawal(ctx context.Context, withdrawalID uint) error {
	w, err := s.withdrawals.FindByID(ctx, withdrawalID)
	if err != nil {
		return domain.ErrWithdrawalNotFound
	}
	if err := w.Approve(); err != nil {
		return err
	}
	return s.withdrawals.Save(ctx, w)
}

// AdminRejectWithdrawal rejects a pending payout and releases the reserved funds
// from BOTH the DB and the Redis cache (the original only unlocked the DB,
// leaving the Redis reservation stuck — fixed here).
func (s *WalletUseCase) AdminRejectWithdrawal(ctx context.Context, withdrawalID uint) error {
	w, err := s.withdrawals.FindByID(ctx, withdrawalID)
	if err != nil {
		return domain.ErrWithdrawalNotFound
	}
	if err := w.Reject(); err != nil {
		return err
	}
	amountUSDT := w.Amount / s.rate.VNDRate()
	if err := s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.wallets.Unlock(ctx, w.UserID, usdt, amountUSDT); err != nil {
			return err
		}
		return s.withdrawals.Save(ctx, w)
	}); err != nil {
		return err
	}
	s.cache.Unlock(ctx, w.UserID, usdt, amountUSDT)
	return nil
}

// ── consumers (event-driven) ─────────────────────────────────────────────────

// ProvisionWallets creates the initial wallet set for a newly-registered user.
// Idempotent: a no-op if the user already has wallets (consumer replay-safe).
func (s *WalletUseCase) ProvisionWallets(ctx context.Context, userID uint, profile string) error {
	if n, err := s.wallets.CountByUser(ctx, userID); err == nil && n > 0 {
		return nil
	}
	vnd, usdtBal, btc, eth := signupBalances(profile)
	wallets := []domain.Wallet{
		{UserID: userID, Currency: "VND", Balance: vnd},
		{UserID: userID, Currency: usdt, Balance: usdtBal},
	}
	for _, coin := range types.DefaultCoins {
		bal := 0.0
		if profile == "admin" && coin.Symbol == "BTC" {
			bal = btc
		}
		if profile == "admin" && coin.Symbol == "ETH" {
			bal = eth
		}
		wallets = append(wallets, domain.Wallet{UserID: userID, Currency: coin.Symbol, Balance: bal})
	}
	return s.wallets.CreateBatch(ctx, wallets)
}

// ApplyBalanceChange is the balance.changed projector: it mirrors a delta that
// already mutated the Redis hot path into the authoritative PostgreSQL row.
func (s *WalletUseCase) ApplyBalanceChange(ctx context.Context, userID uint, currency string, delta float64, reason string) error {
	switch reason {
	case "lock":
		return s.wallets.IncreaseLocked(ctx, userID, currency, delta)
	case "unlock":
		return s.wallets.Unlock(ctx, userID, currency, delta)
	default:
		return s.wallets.UpdateBalance(ctx, userID, currency, delta)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (s *WalletUseCase) publishBalance(ctx context.Context, userID uint, currency string, delta float64, reason, refID string) {
	if s.bus == nil {
		return
	}
	_ = s.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
		UserID: userID, Currency: currency, Delta: delta, Reason: reason, RefID: refID,
	})
}

func (s *WalletUseCase) publishNotification(ctx context.Context, userID uint, typ, title, message string) {
	if s.bus == nil {
		return
	}
	if err := s.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
		UserID: userID, Type: typ, Title: title, Message: message,
	}); err != nil {
		log.Printf("[wallet] notification publish error: %v", err)
	}
}

// signupBalances returns initial (vnd, usdt, btc, eth) balances per SeedProfile.
func signupBalances(profile string) (vnd, usdt, btc, eth float64) {
	switch profile {
	case "system":
		return 0, 0, 0, 0
	case "admin":
		return 100_000_000, 100_000, 1.0, 10.0
	case "demo":
		return 50_000_000, 10_000, 0, 0
	default:
		return 0, 10_000, 0, 0
	}
}
