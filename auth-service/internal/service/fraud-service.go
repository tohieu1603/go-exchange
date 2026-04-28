package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

type FraudService struct {
	db       *gorm.DB
	userRepo repository.UserRepo
	bonusSvc *BonusService
}

func NewFraudService(db *gorm.DB, userRepo repository.UserRepo, bonusSvc *BonusService) *FraudService {
	return &FraudService{db: db, userRepo: userRepo, bonusSvc: bonusSvc}
}

// THRESHOLD constants
const (
	maxTradesPerPairPerHour = 5   // >5 trades between same 2 users = suspicious
	maxTradesPerPairPerDay  = 15  // >15/day = definite farming
	minVolumeForAlert       = 50  // only flag if total volume > $50
)

// OnTradeExecuted is called by Kafka consumer when a trade happens.
// Tracks cross-user trade frequency and triggers fraud detection.
func (s *FraudService) OnTradeExecuted(buyerID, sellerID uint, pair string, amount, total float64) {
	if buyerID == 0 || sellerID == 0 || buyerID == sellerID {
		return // instant-fill (no counterparty) or self-trade impossible
	}

	u1, u2 := model.SortedUserIDs(buyerID, sellerID)
	now := time.Now()

	// Upsert trade pair counter
	var utp model.UserTradePair
	err := s.db.Where("user1_id = ? AND user2_id = ? AND pair = ?", u1, u2, pair).First(&utp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utp = model.UserTradePair{
			User1ID: u1, User2ID: u2, Pair: pair,
			TradeCount: 1, TotalVol: total,
			FirstTrade: now, LastTrade: now,
		}
		s.db.Create(&utp)
	} else if err == nil {
		s.db.Model(&utp).Updates(map[string]interface{}{
			"trade_count": gorm.Expr("trade_count + 1"),
			"total_vol":   gorm.Expr("total_vol + ?", total),
			"last_trade":  now,
		})
		utp.TradeCount++
		utp.TotalVol += total
	}

	// Check thresholds
	if utp.TradeCount >= maxTradesPerPairPerHour && utp.TotalVol >= minVolumeForAlert {
		hoursSinceFirst := now.Sub(utp.FirstTrade).Hours()
		if hoursSinceFirst <= 1 || utp.TradeCount >= maxTradesPerPairPerDay {
			s.flagBonusFarming(u1, u2, pair, utp.TradeCount, utp.TotalVol)
		}
	}

	// Also check same-IP across the two users
	s.checkSameIP(u1, u2)
}

// flagBonusFarming locks accounts + revokes bonuses if either user has active bonus
func (s *FraudService) flagBonusFarming(u1, u2 uint, pair string, tradeCount int, totalVol float64) {
	// Check if already flagged
	var existing int64
	s.db.Model(&model.FraudLog{}).
		Where("fraud_type = ? AND user_ids = ? AND action != ?",
			"BONUS_FARMING", fmt.Sprintf("%d,%d", u1, u2), "DISMISSED").
		Count(&existing)
	if existing > 0 {
		return // already flagged
	}

	// Check if either user has active bonus
	b1, _ := s.bonusSvc.bonusRepo.SumActiveBonus(u1)
	b2, _ := s.bonusSvc.bonusRepo.SumActiveBonus(u2)
	if b1 == 0 && b2 == 0 {
		return // no bonus to farm
	}

	evidence, _ := json.Marshal(map[string]interface{}{
		"user1":      u1, "user2": u2,
		"pair":       pair,
		"tradeCount": tradeCount,
		"totalVol":   totalVol,
		"bonusUser1": b1, "bonusUser2": b2,
	})

	fraudLog := &model.FraudLog{
		UserIDs:     fmt.Sprintf("%d,%d", u1, u2),
		FraudType:   "BONUS_FARMING",
		Description: fmt.Sprintf("Wash trading detected: %d trades on %s, vol=$%.2f. Both accounts locked.", tradeCount, pair, totalVol),
		Evidence:    string(evidence),
		Action:      "ACCOUNTS_LOCKED",
	}
	s.db.Create(fraudLog)

	// Lock both accounts
	_ = s.LockAccount(u1, "Auto-locked: suspected bonus farming wash trading")
	_ = s.LockAccount(u2, "Auto-locked: suspected bonus farming wash trading")

	// Revoke all bonuses
	_ = s.bonusSvc.RevokeUserBonuses(u1)
	_ = s.bonusSvc.RevokeUserBonuses(u2)

	log.Printf("[FRAUD] BONUS_FARMING detected: users=%d,%d pair=%s trades=%d vol=$%.2f → LOCKED", u1, u2, pair, tradeCount, totalVol)
}

// checkSameIP detects multi-account from same IP
func (s *FraudService) checkSameIP(u1, u2 uint) {
	user1, err1 := s.userRepo.FindByID(u1)
	user2, err2 := s.userRepo.FindByID(u2)
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

	// Check if already flagged
	var existing int64
	s.db.Model(&model.FraudLog{}).
		Where("fraud_type = ? AND user_ids = ?", "MULTI_ACCOUNT", fmt.Sprintf("%d,%d", u1, u2)).
		Count(&existing)
	if existing > 0 {
		return
	}

	evidence, _ := json.Marshal(map[string]interface{}{
		"user1": u1, "user2": u2,
		"ip1":      user1.LastLoginIP, "ip2": user2.LastLoginIP,
		"regIp1":   user1.RegisterIP, "regIp2": user2.RegisterIP,
	})
	s.db.Create(&model.FraudLog{
		UserIDs:     fmt.Sprintf("%d,%d", u1, u2),
		FraudType:   "MULTI_ACCOUNT",
		Description: fmt.Sprintf("Same IP detected: %s (login) / %s (register)", user1.LastLoginIP, user1.RegisterIP),
		Evidence:    string(evidence),
		Action:      "FLAGGED",
	})
	log.Printf("[FRAUD] MULTI_ACCOUNT flagged: users=%d,%d ip=%s", u1, u2, user1.LastLoginIP)
}

func (s *FraudService) LockAccount(userID uint, reason string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	user.IsLocked = true
	user.LockReason = reason
	return s.userRepo.Update(nil, user)
}

func (s *FraudService) UnlockAccount(userID uint) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	user.IsLocked = false
	user.LockReason = ""
	return s.userRepo.Update(nil, user)
}

func (s *FraudService) GetFraudLogs(page, size int, search string) ([]model.FraudLog, int64, error) {
	var logs []model.FraudLog
	var total int64
	offset := (page - 1) * size
	q := s.db.Model(&model.FraudLog{})
	if search != "" {
		q = q.Where("user_ids LIKE ? OR fraud_type LIKE ? OR description LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset(offset).Find(&logs).Error
	return logs, total, err
}

func (s *FraudService) UpdateFraudAction(logID uint, action, note string) error {
	validActions := map[string]bool{
		"FLAGGED": true, "ACCOUNTS_LOCKED": true, "BONUS_REVOKED": true, "DISMISSED": true,
	}
	if !validActions[action] {
		return errors.New("invalid action value")
	}
	return s.db.Model(&model.FraudLog{}).Where("id = ?", logID).
		Updates(map[string]interface{}{"action": action, "admin_note": note}).Error
}
