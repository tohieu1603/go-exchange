package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/auth-service/internal/model"
)

// CommissionRate is the share of trading fee the referrer receives.
// 0.20 = 20%. Configurable later via platform_settings.
const CommissionRate = 0.20

// WalletCrediter is the minimal interface ReferralService needs from the
// wallet gRPC client. Defined here (consumer side) to avoid an import cycle
// between service ↔ internal/grpc.
type WalletCrediter interface {
	Credit(ctx context.Context, userID uint, currency string, amount float64) error
}

// ReferralService manages referral codes, bindings, and commission settlement.
//
// Settlement model: IMMEDIATE.
// On every trade.executed, the commission is computed, persisted, and credited
// to the referrer's USDT wallet via wallet-service gRPC. Idempotency is
// guaranteed by a unique index on (trade_id) — replays are no-ops.
type ReferralService struct {
	repo         repository.ReferralRepo
	walletClient WalletCrediter // nil = accrue-only (no payout)
	bus          eventbus.EventPublisher
}

func NewReferralService(repo repository.ReferralRepo, walletClient WalletCrediter, bus eventbus.EventPublisher) *ReferralService {
	return &ReferralService{repo: repo, walletClient: walletClient, bus: bus}
}

// EnsureDefaultCode lazily creates a default referral code for a user.
// Called on first access from MyCode().
func (s *ReferralService) EnsureDefaultCode(userID uint) (*model.ReferralCode, error) {
	if existing, err := s.repo.FindDefaultByUser(userID); err == nil && existing != nil {
		return existing, nil
	}
	for tries := 0; tries < 5; tries++ {
		code, err := newReferralCode()
		if err != nil {
			return nil, err
		}
		if _, err := s.repo.FindCodeByValue(code); err == nil {
			continue // collision, retry
		}
		c := &model.ReferralCode{UserID: userID, Code: code, IsDefault: true}
		if err := s.repo.CreateCode(c); err != nil {
			return nil, err
		}
		return c, nil
	}
	return nil, errors.New("could not generate unique referral code")
}

// BindOnRegister is called from the user.registered consumer. Resolves the
// presented code → referrer; records the immutable Referral row.
// No-op if code is empty, invalid, or self-referral.
func (s *ReferralService) BindOnRegister(refereeID uint, codeUsed string) {
	if codeUsed == "" {
		return
	}
	code, err := s.repo.FindCodeByValue(codeUsed)
	if err != nil || code == nil {
		return
	}
	if code.UserID == refereeID {
		return // can't refer yourself
	}
	// Skip if referee already has a referral row.
	if existing, _ := s.repo.FindReferralByReferee(refereeID); existing != nil && existing.ID != 0 {
		return
	}
	_ = s.repo.CreateReferral(&model.Referral{
		ReferrerID: code.UserID,
		RefereeID:  refereeID,
		Code:       code.Code,
		Tier:       1,
	})
	_ = s.repo.IncrementUsage(code.ID)
}

// OnTradeFee is called from the trade.executed consumer. Credits the referee's
// referrer with CommissionRate × fee, immediately:
//
//   1. Records ReferralCommission row (idempotent on tradeID — replay-safe).
//   2. Calls wallet-service gRPC Credit on the referrer's USDT wallet.
//   3. Publishes balance.changed reason="referral_commission" for projector.
//
// If the commission row already exists (FirstOrCreate hit), the wallet credit
// is SKIPPED — this is the idempotency contract.
func (s *ReferralService) OnTradeFee(refereeID, tradeID uint, currency string, fee float64) {
	if fee <= 0 || tradeID == 0 {
		return
	}
	ref, err := s.repo.FindReferralByReferee(refereeID)
	if err != nil || ref == nil {
		return
	}
	commission := fee * CommissionRate
	row := &model.ReferralCommission{
		ReferrerID: ref.ReferrerID,
		RefereeID:  refereeID,
		TradeID:    tradeID,
		Currency:   currency,
		FeeAmount:  fee,
		Rate:       CommissionRate,
		Commission: commission,
	}
	// FirstOrCreate semantics: if a row for this tradeID already exists,
	// the returned row has its existing ID and we skip payout.
	created, err := s.createCommissionIfNew(row)
	if err != nil {
		log.Printf("[referral] commission persist trade=%d: %v", tradeID, err)
		return
	}
	if !created {
		return // already settled previously
	}

	// Immediate payout — credit referrer's wallet via gRPC.
	if s.walletClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.walletClient.Credit(ctx, ref.ReferrerID, currency, commission); err != nil {
			log.Printf("[referral] payout failed referrer=%d trade=%d: %v",
				ref.ReferrerID, tradeID, err)
			// Commission row remains; an admin tool can reconcile manually.
			return
		}
	}

	// Emit balance.changed so the wallet projector + audit see this delta.
	if s.bus != nil {
		s.bus.Publish(context.Background(), eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
			UserID:   ref.ReferrerID,
			Currency: currency,
			Delta:    commission,
			Reason:   "referral_commission",
			RefID:    fmt.Sprintf("trade-%d", tradeID),
		})
	}
}

// createCommissionIfNew returns (true, nil) when a fresh row was inserted,
// (false, nil) when the unique tradeID already had a row.
func (s *ReferralService) createCommissionIfNew(c *model.ReferralCommission) (bool, error) {
	// Check first to distinguish create-vs-no-op cleanly. Race is acceptable:
	// the unique index will reject the second insert, which we treat as no-op.
	if existing, _ := s.repo.FindCommissionByTrade(c.TradeID); existing != nil && existing.ID != 0 {
		return false, nil
	}
	if err := s.repo.CreateCommission(c); err != nil {
		return false, err
	}
	return true, nil
}

func (s *ReferralService) MyCode(userID uint) (*model.ReferralCode, error) {
	return s.EnsureDefaultCode(userID)
}

func (s *ReferralService) MyStats(userID uint) (map[string]interface{}, error) {
	code, _ := s.EnsureDefaultCode(userID)
	commission, err := s.repo.SumCommissionByUser(userID)
	if err != nil {
		return nil, err
	}
	rows, total, _ := s.repo.ListReferees(userID, 1, 1)
	_ = rows
	return map[string]interface{}{
		"code":             code,
		"totalReferees":    total,
		"totalCommission":  commission,
		"commissionRate":   CommissionRate,
	}, nil
}

func (s *ReferralService) ListReferees(userID uint, page, size int) ([]model.Referral, int64, error) {
	return s.repo.ListReferees(userID, page, size)
}

func (s *ReferralService) ListCommissions(userID uint, page, size int) ([]model.ReferralCommission, int64, error) {
	return s.repo.ListCommissions(userID, page, size)
}

// newReferralCode returns a 6-char alphanumeric code prefixed "MX-".
func newReferralCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Crockford-ish (no 0/O/I/1)
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 6)
	for i := range b {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return fmt.Sprintf("MX-%s", string(out)), nil
}
