package usecase

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/cryptox/auth-service/internal/domain"
	"github.com/redis/go-redis/v9"
)

// FeeTierService maintains 30-day rolling volume per user and resolves
// the current VIP tier (and maker/taker rates).
//
// Volume tracking is incremental: each trade.executed event adds
// `total` to the user's volume row. A periodic decay job subtracts
// volume older than 30 days (simplified: we apply linear decay).
//
// For a no-op simple implementation, this service uses INCREMENT-only
// and exposes Redis caching for hot reads from trading-application.
type FeeTierService struct {
	repo FeeTierRepo
	rdb  *redis.Client
}

func NewFeeTierService(repo FeeTierRepo, rdb *redis.Client) *FeeTierService {
	return &FeeTierService{repo: repo, rdb: rdb}
}

// SeedDefaults populates the fee_tiers table on first boot.
func (s *FeeTierService) SeedDefaults() error { return s.repo.SeedDefaults(context.Background()) }

// ListTiers returns all configured tiers.
func (s *FeeTierService) ListTiers() ([]domain.FeeTier, error) {
	return s.repo.ListAll(context.Background())
}

// AddVolume is called by the trade event consumer (auth-service).
// Adds USD volume to user's rolling counter and updates tier cache.
func (s *FeeTierService) AddVolume(userID uint, total float64) error {
	if total <= 0 {
		return nil
	}
	if err := s.repo.IncrementVolume(context.Background(), userID, total); err != nil {
		return err
	}
	// Update Redis cache for fast lookups by trading-application.
	go s.refreshUserCache(userID)
	return nil
}

// MyTier returns a user's current tier with rates and progress.
type MyTierView struct {
	Tier       domain.FeeTier  `json:"tier"`
	Volume30d  float64        `json:"volume30d"`
	NextTier   *domain.FeeTier `json:"nextTier,omitempty"`
	NextNeeded float64        `json:"nextNeeded,omitempty"`
}

func (s *FeeTierService) MyTier(userID uint) (*MyTierView, error) {
	tiers, err := s.repo.ListAll(context.Background())
	if err != nil {
		return nil, err
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].Level < tiers[j].Level })

	v, _ := s.repo.GetUserVolume(context.Background(), userID)
	var volume float64
	if v != nil {
		volume = v.Volume
	}
	current := tiers[0]
	var next *domain.FeeTier
	for i, t := range tiers {
		if volume >= t.MinVolume {
			current = t
			if i+1 < len(tiers) {
				nxt := tiers[i+1]
				next = &nxt
			}
		}
	}
	view := &MyTierView{Tier: current, Volume30d: volume, NextTier: next}
	if next != nil {
		view.NextNeeded = next.MinVolume - volume
	}
	return view, nil
}

// Rates resolves (maker, taker) for a user via Redis cache; falls back to DB.
// Used by trading-service via gRPC.
func (s *FeeTierService) Rates(userID uint) (float64, float64) {
	ctx := context.Background()
	mk := fmt.Sprintf("fee_tier:%d:maker", userID)
	tk := fmt.Sprintf("fee_tier:%d:taker", userID)

	if maker, err := s.rdb.Get(ctx, mk).Float64(); err == nil {
		if taker, err := s.rdb.Get(ctx, tk).Float64(); err == nil {
			return maker, taker
		}
	}
	// Cache miss — compute, cache 5 min.
	view, err := s.MyTier(userID)
	if err != nil || view == nil {
		return 0.001, 0.001
	}
	s.rdb.Set(ctx, mk, view.Tier.MakerFee, 5*time.Minute)
	s.rdb.Set(ctx, tk, view.Tier.TakerFee, 5*time.Minute)
	return view.Tier.MakerFee, view.Tier.TakerFee
}

func (s *FeeTierService) refreshUserCache(userID uint) {
	view, err := s.MyTier(userID)
	if err != nil || view == nil {
		return
	}
	ctx := context.Background()
	s.rdb.Set(ctx, fmt.Sprintf("fee_tier:%d:maker", userID), view.Tier.MakerFee, 5*time.Minute)
	s.rdb.Set(ctx, fmt.Sprintf("fee_tier:%d:taker", userID), view.Tier.TakerFee, 5*time.Minute)
}
