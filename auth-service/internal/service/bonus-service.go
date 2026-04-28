package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/auth-service/internal/model"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type BonusService struct {
	bonusRepo repository.BonusRepo
	userRepo  repository.UserRepo
	db        *gorm.DB
	rdb       *redis.Client
}

func NewBonusService(bonusRepo repository.BonusRepo, userRepo repository.UserRepo, db *gorm.DB, rdb *redis.Client) *BonusService {
	return &BonusService{bonusRepo: bonusRepo, userRepo: userRepo, db: db, rdb: rdb}
}

// syncBonusCache writes the user's total active bonus (USDT) to Redis.
// Key: user_bonus_remaining:{userID} — read by wallet-service at withdrawal.
func (s *BonusService) syncBonusCache(userID uint) {
	if s.rdb == nil {
		return
	}
	total, err := s.bonusRepo.SumActiveBonus(userID)
	if err != nil {
		return
	}
	s.rdb.Set(context.Background(), fmt.Sprintf("user_bonus_remaining:%d", userID), total, 0)
}

type CreatePromotionReq struct {
	Name           string    `json:"name" binding:"required"`
	Description    string    `json:"description"`
	BonusPercent   float64   `json:"bonusPercent" binding:"required"`
	MaxBonusAmount float64   `json:"maxBonusAmount" binding:"required"`
	TargetType     string    `json:"targetType" binding:"required"` // ALL, SPECIFIC_USERS
	TargetUserIDs  string    `json:"targetUserIds"`
	TriggerType    string    `json:"triggerType" binding:"required"` // ON_DEPOSIT, MANUAL
	MinDeposit     float64   `json:"minDeposit"`
	StartAt        time.Time `json:"startAt" binding:"required"`
	EndAt          time.Time `json:"endAt" binding:"required"`
}

func (s *BonusService) CreatePromotion(req CreatePromotionReq) (*model.BonusPromotion, error) {
	if req.BonusPercent < 10 || req.BonusPercent > 100 {
		return nil, errors.New("bonusPercent must be between 10 and 100")
	}
	if req.MaxBonusAmount <= 0 {
		return nil, errors.New("maxBonusAmount must be greater than 0")
	}
	if !req.StartAt.Before(req.EndAt) {
		return nil, errors.New("startAt must be before endAt")
	}
	promo := &model.BonusPromotion{
		Name:           req.Name,
		Description:    req.Description,
		BonusPercent:   req.BonusPercent,
		MaxBonusAmount: req.MaxBonusAmount,
		TargetType:     req.TargetType,
		TargetUserIDs:  req.TargetUserIDs,
		TriggerType:    req.TriggerType,
		MinDeposit:     req.MinDeposit,
		IsActive:       true,
		StartAt:        req.StartAt,
		EndAt:          req.EndAt,
	}
	if err := s.bonusRepo.CreatePromotion(nil, promo); err != nil {
		return nil, fmt.Errorf("create promotion: %w", err)
	}
	return promo, nil
}

func (s *BonusService) ListPromotions() ([]model.BonusPromotion, error) {
	return s.bonusRepo.FindAllPromotions()
}

func (s *BonusService) TogglePromotion(id uint, active bool) error {
	promo, err := s.bonusRepo.FindPromotionByID(id)
	if err != nil {
		return fmt.Errorf("promotion not found: %w", err)
	}
	promo.IsActive = active
	return s.bonusRepo.UpdatePromotion(nil, promo)
}

func (s *BonusService) ApplyBonusOnDeposit(userID uint, depositAmount float64) (*model.UserBonus, error) {
	now := time.Now()
	promos, err := s.bonusRepo.FindActivePromotions()
	if err != nil {
		return nil, err
	}

	for _, promo := range promos {
		if promo.TriggerType != "ON_DEPOSIT" {
			continue
		}
		if now.Before(promo.StartAt) || now.After(promo.EndAt) {
			continue
		}
		if depositAmount < promo.MinDeposit {
			continue
		}
		if promo.TargetType == "SPECIFIC_USERS" && !containsUserID(promo.TargetUserIDs, userID) {
			continue
		}

		bonusAmt := depositAmount * promo.BonusPercent / 100
		if bonusAmt > promo.MaxBonusAmount {
			bonusAmt = promo.MaxBonusAmount
		}

		bonus := &model.UserBonus{
			UserID:      userID,
			PromotionID: promo.ID,
			BonusAmount: bonusAmt,
			UsedAmount:  0,
			Status:      "ACTIVE",
		}
		if err := s.bonusRepo.CreateUserBonus(nil, bonus); err != nil {
			return nil, fmt.Errorf("create user bonus: %w", err)
		}
		s.syncBonusCache(userID)
		return bonus, nil
	}
	return nil, nil
}

func (s *BonusService) GetUserBonus(userID uint) (float64, []model.UserBonus, error) {
	total, err := s.bonusRepo.SumActiveBonus(userID)
	if err != nil {
		return 0, nil, err
	}
	bonuses, err := s.bonusRepo.FindUserBonuses(userID)
	return total, bonuses, err
}

func (s *BonusService) ConsumeBonus(userID uint, amount float64) error {
	bonuses, err := s.bonusRepo.FindActiveUserBonuses(userID)
	if err != nil {
		return err
	}
	remaining := amount
	for i := range bonuses {
		if remaining <= 0 {
			break
		}
		avail := bonuses[i].RemainingBonus()
		if avail <= 0 {
			continue
		}
		deduct := avail
		if deduct > remaining {
			deduct = remaining
		}
		bonuses[i].UsedAmount += deduct
		if bonuses[i].UsedAmount >= bonuses[i].BonusAmount {
			bonuses[i].Status = "USED"
		}
		if err := s.bonusRepo.UpdateUserBonus(nil, &bonuses[i]); err != nil {
			return fmt.Errorf("update bonus %d: %w", bonuses[i].ID, err)
		}
		remaining -= deduct
	}
	s.syncBonusCache(userID)
	return nil
}

func (s *BonusService) RevokeUserBonuses(userID uint) error {
	bonuses, err := s.bonusRepo.FindActiveUserBonuses(userID)
	if err != nil {
		return err
	}
	for i := range bonuses {
		bonuses[i].Status = "REVOKED"
		if err := s.bonusRepo.UpdateUserBonus(nil, &bonuses[i]); err != nil {
			return fmt.Errorf("revoke bonus %d: %w", bonuses[i].ID, err)
		}
	}
	s.syncBonusCache(userID)
	return nil
}

func containsUserID(csv string, userID uint) bool {
	idStr := strconv.FormatUint(uint64(userID), 10)
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) == idStr {
			return true
		}
	}
	return false
}
