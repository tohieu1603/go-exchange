package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/wallet-service/internal/model"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/utils"
	"github.com/cryptox/wallet-service/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// platformSettingsKey matches the key written by auth-service settings-service.
const platformSettingsKey = "platform_settings"

// VND/USDT exchange rate (cached, refreshed every 5 min)
var (
	cachedRate     float64 = 25500
	rateMu         sync.RWMutex
	rateLastUpdate time.Time
)

type SepayConfig struct {
	BankCode    string
	BankAccount string
	AccountName string
}

type WalletService struct {
	db             *gorm.DB
	walletRepo     repository.WalletRepo
	depositRepo    repository.DepositRepo
	withdrawalRepo repository.WithdrawalRepo
	rdb            *redis.Client
	sepay          SepayConfig
	balCache       *redisutil.BalanceCache
	bus            eventbus.EventPublisher
}

func NewWalletService(
	walletRepo repository.WalletRepo,
	depositRepo repository.DepositRepo,
	withdrawalRepo repository.WithdrawalRepo,
	db *gorm.DB,
	rdb *redis.Client,
	sepay SepayConfig,
	bus eventbus.EventPublisher,
) *WalletService {
	ws := &WalletService{
		db:             db,
		walletRepo:     walletRepo,
		depositRepo:    depositRepo,
		withdrawalRepo: withdrawalRepo,
		rdb:            rdb,
		sepay:          sepay,
		balCache:       redisutil.NewBalanceCache(rdb),
		bus:            bus,
	}
	go ws.refreshRateLoop()
	return ws
}

func GetVNDRate() float64 {
	rateMu.RLock()
	defer rateMu.RUnlock()
	return cachedRate
}

func (s *WalletService) refreshRateLoop() {
	s.fetchRate()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.fetchRate()
	}
}

func (s *WalletService) fetchRate() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://open.er-api.com/v6/latest/USD")
	if err != nil {
		log.Printf("Exchange rate fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Exchange rate decode error: %v", err)
		return
	}
	if vnd, ok := result.Rates["VND"]; ok && vnd > 0 {
		rateMu.Lock()
		cachedRate = vnd
		rateLastUpdate = time.Now()
		rateMu.Unlock()
		log.Printf("Exchange rate updated: 1 USDT = %.0f VND", vnd)
	}
}

func (s *WalletService) EnsureBalanceCached(userID uint, currency string) {
	ctx := context.Background()
	if _, ok := s.balCache.GetBalance(ctx, userID, currency); ok {
		return
	}
	wallet, err := s.walletRepo.FindByUserAndCurrency(userID, currency)
	if err != nil {
		return
	}
	s.balCache.LoadFromDB(ctx, userID, currency, wallet.Balance, wallet.LockedBalance)
}

func (s *WalletService) GetBalances(userID uint) ([]model.Wallet, error) {
	return s.walletRepo.FindAllByUser(userID)
}

// getPlatformSettings reads PlatformSettings from Redis (written by auth-service).
// Falls back to defaults if the key is absent.
func (s *WalletService) getPlatformSettings() types.PlatformSettings {
	raw, err := s.rdb.Get(context.Background(), platformSettingsKey).Bytes()
	if err == nil {
		var ps types.PlatformSettings
		if json.Unmarshal(raw, &ps) == nil {
			return ps
		}
	}
	return types.DefaultPlatformSettings()
}

func (s *WalletService) CreateDeposit(userID uint, amount float64) (*model.Deposit, error) {
	// Check account lock
	locked, _ := s.rdb.Get(context.Background(), fmt.Sprintf("user_locked:%d", userID)).Result()
	if locked == "true" {
		return nil, errors.New("account is locked")
	}

	ps := s.getPlatformSettings()

	if ps.MinDeposit > 0 && amount < ps.MinDeposit {
		return nil, fmt.Errorf("minimum deposit: %.0f VND", ps.MinDeposit)
	}
	if ps.MaxDeposit > 0 && amount > ps.MaxDeposit {
		return nil, fmt.Errorf("maximum deposit: %.0f VND", ps.MaxDeposit)
	}

	rate := GetVNDRate()
	amountUSDT := amount / rate
	// Apply deposit fee
	if ps.DepositFeePercent > 0 {
		amountUSDT = amountUSDT * (1 - ps.DepositFeePercent/100)
	}

	orderCode := fmt.Sprintf("DEP-%d-%d", userID, time.Now().UnixMilli())
	qrURL := utils.GenerateSepayQR(s.sepay.BankAccount, s.sepay.BankCode, amount, orderCode)

	deposit := &model.Deposit{
		UserID: userID, Amount: amount, AmountUSDT: amountUSDT,
		ExchangeRate: rate, Currency: "VND", Method: "BANK_TRANSFER",
		Status: "PENDING", OrderCode: orderCode, QRCodeURL: qrURL,
	}
	err := s.depositRepo.Create(nil, deposit)
	return deposit, err
}

func (s *WalletService) ConfirmDeposit(orderCode string) error {
	deposit, err := s.depositRepo.FindByOrderCode(orderCode)
	if err != nil || deposit.Status != "PENDING" {
		return errors.New("deposit not found or already confirmed")
	}

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.walletRepo.Upsert(tx, deposit.UserID, "USDT", deposit.AmountUSDT); err != nil {
			return err
		}
		deposit.Status = "CONFIRMED"
		return s.depositRepo.Save(tx, deposit)
	})
	if txErr != nil {
		return txErr
	}

	// Credit Redis cache AFTER DB tx committed (no ghost balance on rollback)
	s.balCache.Credit(context.Background(), deposit.UserID, "USDT", deposit.AmountUSDT)
	s.publishDepositNotification(deposit)
	return nil
}

func (s *WalletService) publishDepositNotification(deposit *model.Deposit) {
	if s.bus == nil {
		return
	}
	event := eventbus.NotificationEvent{
		UserID:  deposit.UserID,
		Type:    "DEPOSIT_CONFIRMED",
		Title:   "Deposit Confirmed",
		Message: fmt.Sprintf("$%.2f USDT deposited", deposit.AmountUSDT),
	}
	ctx := context.Background()
	if err := s.bus.Publish(ctx, eventbus.TopicNotificationCreated, event); err != nil {
		log.Printf("[wallet] notification publish error: %v", err)
	}
}

func (s *WalletService) EnsureWallet(userID uint, currency string) {
	s.db.Exec(`
		INSERT INTO wallets (user_id, currency, balance, locked_balance, updated_at)
		VALUES (?, ?, 0, 0, NOW())
		ON CONFLICT (user_id, currency) DO NOTHING
	`, userID, currency)
}

func (s *WalletService) UpdateBalance(tx *gorm.DB, userID uint, currency string, delta float64) error {
	if delta == 0 {
		return nil
	}
	ctx := context.Background()
	s.EnsureBalanceCached(userID, currency)
	if delta < 0 {
		if _, err := s.balCache.Deduct(ctx, userID, currency, -delta); err != nil {
			return errors.New("insufficient balance")
		}
	} else {
		s.balCache.Credit(ctx, userID, currency, delta)
	}

	if err := s.walletRepo.UpdateBalance(tx, userID, currency, delta); err != nil {
		if delta < 0 {
			s.balCache.Credit(ctx, userID, currency, -delta)
		} else {
			s.balCache.Deduct(ctx, userID, currency, delta)
		}
		return err
	}
	return nil
}

func (s *WalletService) LockBalance(tx *gorm.DB, userID uint, currency string, amount float64) error {
	ctx := context.Background()
	s.EnsureBalanceCached(userID, currency)

	if err := s.balCache.Lock(ctx, userID, currency, amount); err != nil {
		return errors.New("insufficient balance")
	}

	if err := s.walletRepo.LockBalance(tx, userID, currency, amount); err != nil {
		s.balCache.Unlock(ctx, userID, currency, amount)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("insufficient balance")
		}
		return err
	}
	return nil
}

func (s *WalletService) UnlockBalance(tx *gorm.DB, userID uint, currency string, amount float64) error {
	ctx := context.Background()
	s.balCache.Unlock(ctx, userID, currency, amount)
	return s.walletRepo.UnlockBalance(tx, userID, currency, amount)
}

// Redis-only hot path ops.
//
// Each mutation publishes a balance.changed event so the DB projector + audit
// see the same delta. Without this, callers (futures, etc.) that hit Redis
// via gRPC would not be projected into PostgreSQL.

func (s *WalletService) UpdateBalanceRedis(ctx context.Context, userID uint, currency string, delta float64) error {
	if delta == 0 {
		return nil
	}
	s.EnsureBalanceCached(userID, currency)
	if delta < 0 {
		if _, err := s.balCache.Deduct(ctx, userID, currency, -delta); err != nil {
			return errors.New("insufficient balance")
		}
	} else {
		s.balCache.Credit(ctx, userID, currency, delta)
	}
	s.publishBalanceChange(ctx, userID, currency, delta, "grpc", "")
	return nil
}

func (s *WalletService) LockBalanceRedis(ctx context.Context, userID uint, currency string, amount float64) error {
	s.EnsureBalanceCached(userID, currency)
	if err := s.balCache.Lock(ctx, userID, currency, amount); err != nil {
		return errors.New("insufficient balance")
	}
	// Lock changes locked-balance only — emit a dedicated reason so the
	// projector can update the locked column (positive amount = funds locked).
	s.publishBalanceChange(ctx, userID, currency, amount, "lock", "")
	return nil
}

func (s *WalletService) UnlockBalanceRedis(ctx context.Context, userID uint, currency string, amount float64) error {
	s.balCache.Unlock(ctx, userID, currency, amount)
	s.publishBalanceChange(ctx, userID, currency, amount, "unlock", "")
	return nil
}

// publishBalanceChange swallows publish errors — Redis already mutated, a
// missed event will be reconciled at next cold-start sync.
func (s *WalletService) publishBalanceChange(ctx context.Context, userID uint, currency string, delta float64, reason, refID string) {
	if s.bus == nil {
		return
	}
	_ = s.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
		UserID: userID, Currency: currency, Delta: delta, Reason: reason, RefID: refID,
	})
}

func (s *WalletService) CheckBalanceRedis(ctx context.Context, userID uint, currency string, needed float64) error {
	s.EnsureBalanceCached(userID, currency)
	bal, ok := s.balCache.GetBalance(ctx, userID, currency)
	if !ok {
		return errors.New("balance not found")
	}
	locked := s.balCache.GetLocked(ctx, userID, currency)
	available := bal - locked
	if available < needed {
		return errors.New("insufficient balance")
	}
	return nil
}

func (s *WalletService) GetBalanceRedis(ctx context.Context, userID uint, currency string) (float64, float64) {
	s.EnsureBalanceCached(userID, currency)
	bal, _ := s.balCache.GetBalance(ctx, userID, currency)
	locked := s.balCache.GetLocked(ctx, userID, currency)
	return bal, locked
}

func (s *WalletService) GetDepositHistory(userID uint, page, size int) ([]model.Deposit, int64, error) {
	return s.depositRepo.FindByUser(userID, page, size)
}

func (s *WalletService) GetWithdrawalHistory(userID uint, page, size int) ([]model.Withdrawal, int64, error) {
	return s.withdrawalRepo.FindByUser(userID, page, size)
}

func (s *WalletService) CreateWithdrawal(userID uint, amountUSDT float64, bankCode, bankAccount, accountName string) (*model.Withdrawal, error) {
	ctx := context.Background()

	// Check account lock
	locked, _ := s.rdb.Get(ctx, fmt.Sprintf("user_locked:%d", userID)).Result()
	if locked == "true" {
		return nil, errors.New("account is locked")
	}

	rate := GetVNDRate()
	vndAmount := amountUSDT * rate

	ps := s.getPlatformSettings()
	if ps.MinWithdraw > 0 && vndAmount < ps.MinWithdraw {
		return nil, fmt.Errorf("minimum withdrawal: %.0f VND", ps.MinWithdraw)
	}
	if ps.MaxWithdraw > 0 && vndAmount > ps.MaxWithdraw {
		return nil, fmt.Errorf("maximum withdrawal: %.0f VND", ps.MaxWithdraw)
	}

	// Enforce bonus withdrawal cap:
	// withdrawable USDT = total_balance - active_bonus_remaining
	bonusRemaining, _ := s.rdb.Get(ctx, fmt.Sprintf("user_bonus_remaining:%d", userID)).Float64()
	if bonusRemaining > 0 {
		s.EnsureBalanceCached(userID, "USDT")
		totalBal, _ := s.balCache.GetBalance(ctx, userID, "USDT")
		lockedBal := s.balCache.GetLocked(ctx, userID, "USDT")
		available := totalBal - lockedBal
		withdrawable := available - bonusRemaining
		if withdrawable < 0 {
			withdrawable = 0
		}
		if amountUSDT > withdrawable {
			return nil, fmt.Errorf("withdrawal limited to %.4f USDT (balance minus active bonus)", withdrawable)
		}
	}

	var withdrawal *model.Withdrawal
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.LockBalance(tx, userID, "USDT", amountUSDT); err != nil {
			return err
		}
		withdrawal = &model.Withdrawal{
			UserID:      userID,
			Amount:      vndAmount,
			Currency:    "VND",
			BankCode:    bankCode,
			BankAccount: bankAccount,
			AccountName: accountName,
			Status:      "PENDING",
		}
		return s.withdrawalRepo.Create(tx, withdrawal)
	})
	if err != nil {
		return nil, err
	}

	created, err := s.withdrawalRepo.FindLatestPendingByUser(userID)
	if err != nil {
		return withdrawal, nil
	}
	return created, nil
}

// AdminListDeposits returns all deposits with optional search/status filter, paginated.
func (s *WalletService) AdminListDeposits(page, size int, search, status string) ([]model.Deposit, int64, error) {
	var deposits []model.Deposit
	var total int64
	offset := (page - 1) * size

	q := s.db.Model(&model.Deposit{})
	if status != "" {
		q = q.Where("deposits.status = ?", status)
	}
	if search != "" {
		like := "%" + search + "%"
		q = q.Joins("JOIN users ON users.id = deposits.user_id").
			Where("users.email ILIKE ?", like)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("deposits.created_at DESC").Limit(size).Offset(offset).Find(&deposits).Error
	return deposits, total, err
}

// AdminRejectDeposit rejects a pending deposit by setting status to FAILED.
func (s *WalletService) AdminRejectDeposit(depositID uint) error {
	result := s.db.Model(&model.Deposit{}).
		Where("id = ? AND status = ?", depositID, "PENDING").
		Update("status", "FAILED")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("deposit not found or not in PENDING status")
	}
	return nil
}

// AdminListWithdrawals returns all withdrawals with optional search/status filter, paginated.
func (s *WalletService) AdminListWithdrawals(page, size int, search, status string) ([]model.Withdrawal, int64, error) {
	var withdrawals []model.Withdrawal
	var total int64
	offset := (page - 1) * size

	q := s.db.Model(&model.Withdrawal{})
	if status != "" {
		q = q.Where("withdrawals.status = ?", status)
	}
	if search != "" {
		like := "%" + search + "%"
		q = q.Joins("JOIN users ON users.id = withdrawals.user_id").
			Where("users.email ILIKE ? OR withdrawals.bank_account ILIKE ?", like, like)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("withdrawals.created_at DESC").Limit(size).Offset(offset).Find(&withdrawals).Error
	return withdrawals, total, err
}

// AdminApproveWithdrawal approves a pending withdrawal.
func (s *WalletService) AdminApproveWithdrawal(withdrawalID uint) error {
	result := s.db.Model(&model.Withdrawal{}).
		Where("id = ? AND status = ?", withdrawalID, "PENDING").
		Update("status", "APPROVED")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("withdrawal not found or not in PENDING status")
	}
	return nil
}

// AdminRejectWithdrawal rejects a pending withdrawal and refunds the locked balance.
func (s *WalletService) AdminRejectWithdrawal(withdrawalID uint) error {
	withdrawal, err := s.withdrawalRepo.FindByID(withdrawalID)
	if err != nil {
		return errors.New("withdrawal not found")
	}
	if withdrawal.Status != "PENDING" {
		return errors.New("withdrawal is not in PENDING status")
	}

	rate := GetVNDRate()
	amountUSDT := withdrawal.Amount / rate

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.walletRepo.UnlockBalance(tx, withdrawal.UserID, "USDT", amountUSDT); err != nil {
			return err
		}
		withdrawal.Status = "REJECTED"
		return s.withdrawalRepo.Save(tx, withdrawal)
	})
}
