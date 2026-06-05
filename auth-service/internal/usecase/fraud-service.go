package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/auth-service/internal/domain"
)

type FraudService struct {
	fraud    FraudRepo
	userRepo UserRepo
	bonusSvc *BonusService
}

func NewFraudService(fraud FraudRepo, userRepo UserRepo, bonusSvc *BonusService) *FraudService {
	return &FraudService{fraud: fraud, userRepo: userRepo, bonusSvc: bonusSvc}
}

// THRESHOLD constants
const (
	maxTradesPerPairPerHour = 5  // >5 trades between same 2 users = suspicious
	maxTradesPerPairPerDay  = 15 // >15/day = definite farming
	minVolumeForAlert       = 50 // only flag if total volume > $50
)

// OnTradeExecuted is called by the trade.executed consumer. It tracks cross-user
// trade frequency and triggers fraud detection.
func (s *FraudService) OnTradeExecuted(buyerID, sellerID uint, pair string, amount, total float64) {
	if buyerID == 0 || sellerID == 0 || buyerID == sellerID {
		return // instant-fill (no counterparty) or self-trade impossible
	}
	ctx := context.Background()
	u1, u2 := domain.SortedUserIDs(buyerID, sellerID)

	counters, err := s.fraud.UpsertTradePair(ctx, u1, u2, pair, total)
	if err != nil {
		log.Printf("[FRAUD] upsert trade pair error: %v", err)
		return
	}

	if counters.TradeCount >= maxTradesPerPairPerHour && counters.TotalVol >= minVolumeForAlert {
		hoursSinceFirst := time.Now().Sub(counters.FirstTrade).Hours()
		if hoursSinceFirst <= 1 || counters.TradeCount >= maxTradesPerPairPerDay {
			s.flagBonusFarming(ctx, u1, u2, pair, counters.TradeCount, counters.TotalVol)
		}
	}

	s.checkSameIP(ctx, u1, u2)
}

// flagBonusFarming locks accounts + revokes bonuses if either user has active bonus.
func (s *FraudService) flagBonusFarming(ctx context.Context, u1, u2 uint, pair string, tradeCount int, totalVol float64) {
	ids := fmt.Sprintf("%d,%d", u1, u2)
	if n, _ := s.fraud.CountActiveByTypeUsers(ctx, "BONUS_FARMING", ids); n > 0 {
		return // already flagged
	}

	b1, _ := s.bonusSvc.bonusRepo.SumActiveBonus(ctx, u1)
	b2, _ := s.bonusSvc.bonusRepo.SumActiveBonus(ctx, u2)
	if b1 == 0 && b2 == 0 {
		return // no bonus to farm
	}

	evidence, _ := json.Marshal(map[string]interface{}{
		"user1": u1, "user2": u2, "pair": pair,
		"tradeCount": tradeCount, "totalVol": totalVol, "bonusUser1": b1, "bonusUser2": b2,
	})
	_ = s.fraud.CreateFraudLog(ctx, &domain.FraudLog{
		UserIDs:     ids,
		FraudType:   "BONUS_FARMING",
		Description: fmt.Sprintf("Wash trading detected: %d trades on %s, vol=$%.2f. Both accounts locked.", tradeCount, pair, totalVol),
		Evidence:    string(evidence),
		Action:      "ACCOUNTS_LOCKED",
	})

	_ = s.LockAccount(u1, "Auto-locked: suspected bonus farming wash trading")
	_ = s.LockAccount(u2, "Auto-locked: suspected bonus farming wash trading")
	_ = s.bonusSvc.RevokeUserBonuses(u1)
	_ = s.bonusSvc.RevokeUserBonuses(u2)

	log.Printf("[FRAUD] BONUS_FARMING detected: users=%d,%d pair=%s trades=%d vol=$%.2f → LOCKED", u1, u2, pair, tradeCount, totalVol)
}

// checkSameIP detects multi-account from the same IP.
func (s *FraudService) checkSameIP(ctx context.Context, u1, u2 uint) {
	user1, err1 := s.userRepo.FindByID(ctx, u1)
	user2, err2 := s.userRepo.FindByID(ctx, u2)
	if err1 != nil || err2 != nil {
		return
	}

	sameIP := false
	if user1.LastLoginIP != "" && user1.LastLoginIP == user2.LastLoginIP {
		sameIP = true
	}
	if user1.RegisterIP != "" && user1.RegisterIP == user2.RegisterIP {
		sameIP = true
	}
	if !sameIP {
		return
	}

	ids := fmt.Sprintf("%d,%d", u1, u2)
	if n, _ := s.fraud.CountByTypeUsers(ctx, "MULTI_ACCOUNT", ids); n > 0 {
		return
	}
	evidence, _ := json.Marshal(map[string]interface{}{
		"user1": u1, "user2": u2,
		"ip1": user1.LastLoginIP, "ip2": user2.LastLoginIP,
		"regIp1": user1.RegisterIP, "regIp2": user2.RegisterIP,
	})
	_ = s.fraud.CreateFraudLog(ctx, &domain.FraudLog{
		UserIDs:     ids,
		FraudType:   "MULTI_ACCOUNT",
		Description: fmt.Sprintf("Same IP detected: %s (login) / %s (register)", user1.LastLoginIP, user1.RegisterIP),
		Evidence:    string(evidence),
		Action:      "FLAGGED",
	})
	log.Printf("[FRAUD] MULTI_ACCOUNT flagged: users=%d,%d ip=%s", u1, u2, user1.LastLoginIP)
}

func (s *FraudService) LockAccount(userID uint, reason string) error {
	ctx := context.Background()
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	user.IsLocked = true
	user.LockReason = reason
	return s.userRepo.Update(ctx, user)
}

func (s *FraudService) UnlockAccount(userID uint) error {
	ctx := context.Background()
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	user.IsLocked = false
	user.LockReason = ""
	return s.userRepo.Update(ctx, user)
}

func (s *FraudService) GetFraudLogs(page, size int, search string) ([]domain.FraudLog, int64, error) {
	return s.fraud.ListFraudLogs(context.Background(), search, page, size)
}

func (s *FraudService) UpdateFraudAction(logID uint, action, note string) error {
	validActions := map[string]bool{
		"FLAGGED": true, "ACCOUNTS_LOCKED": true, "BONUS_REVOKED": true, "DISMISSED": true,
	}
	if !validActions[action] {
		return errors.New("invalid action value")
	}
	return s.fraud.UpdateFraudAction(context.Background(), logID, action, note)
}
