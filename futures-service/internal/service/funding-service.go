package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/futures-service/internal/repository"
	svcgrpc "github.com/cryptox/futures-service/internal/grpc"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/futures-service/internal/model"
	"github.com/redis/go-redis/v9"
)

// FundingService computes and settles perpetual funding payments.
//
// Funding interval: every 8h (00:00, 08:00, 16:00 UTC).
// Rate is the premium of mark over index price (capped ±0.5%).
// Sign convention: positive rate → LONG pays SHORT.
//
// At settlement:
//   payment = signed(rate) × position.size × markPrice
//   LONG:  balance change = −rate × notional
//   SHORT: balance change = +rate × notional
type FundingService struct {
	repo         repository.FundingRepo
	posRepo      repository.PositionRepo
	walletClient *svcgrpc.WalletClient
	rdb          *redis.Client
	bus          eventbus.EventPublisher
}

func NewFundingService(
	repo repository.FundingRepo,
	posRepo repository.PositionRepo,
	walletClient *svcgrpc.WalletClient,
	rdb *redis.Client,
	bus eventbus.EventPublisher,
) *FundingService {
	return &FundingService{
		repo: repo, posRepo: posRepo, walletClient: walletClient,
		rdb: rdb, bus: bus,
	}
}

// MaxFundingRate caps the per-interval rate at ±0.5% to prevent extreme moves.
const MaxFundingRate = 0.005

// FundingInterval is the settlement period (8h is the industry default).
const FundingInterval = 8 * time.Hour

// Start launches a goroutine that ticks at each FundingInterval boundary
// and runs Settle() across all active perpetual pairs.
func (s *FundingService) Start(ctx context.Context, pairs []string) {
	go func() {
		// Wait until the next funding boundary.
		next := nextFundingTime(time.Now())
		for {
			d := time.Until(next)
			if d < 0 {
				d = 0
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
			if err := s.Settle(ctx, pairs, next); err != nil {
				log.Printf("[funding] settle error: %v", err)
			}
			next = nextFundingTime(time.Now())
		}
	}()
	log.Printf("[funding] scheduler started; next settlement at %s", nextFundingTime(time.Now()).UTC())
}

// Settle computes the funding rate for each pair, records a FundingRate row,
// and applies a FundingPayment to every OPEN position on that pair.
func (s *FundingService) Settle(ctx context.Context, pairs []string, settledAt time.Time) error {
	for _, pair := range pairs {
		mark, ok := s.markPrice(ctx, pair)
		if !ok || mark <= 0 {
			continue
		}
		index, ok := s.indexPrice(ctx, pair)
		if !ok || index <= 0 {
			index = mark
		}

		// Premium-based rate, capped.
		rate := (mark - index) / index
		if rate > MaxFundingRate {
			rate = MaxFundingRate
		}
		if rate < -MaxFundingRate {
			rate = -MaxFundingRate
		}

		fr := &model.FundingRate{
			Pair:       pair,
			Rate:       rate,
			IndexPrice: index,
			MarkPrice:  mark,
			Interval:   "8h",
			SettledAt:  settledAt.UTC(),
			CreatedAt:  time.Now(),
		}
		if err := s.repo.CreateRate(fr); err != nil {
			log.Printf("[funding] save rate %s: %v", pair, err)
			continue
		}

		// Cache latest rate for fast UI lookup.
		s.rdb.Set(ctx, fmt.Sprintf("funding:%s", pair), rate, 9*time.Hour)

		// Apply to every OPEN position on this pair.
		positions, err := s.posRepo.FindAllOpen()
		if err != nil {
			log.Printf("[funding] load open positions: %v", err)
			continue
		}
		for _, p := range positions {
			if p.Pair != pair {
				continue
			}
			if err := s.applyPayment(ctx, &p, fr); err != nil {
				log.Printf("[funding] apply pos=%d: %v", p.ID, err)
			}
		}
	}
	return nil
}

func (s *FundingService) applyPayment(ctx context.Context, p *model.FuturesPosition, fr *model.FundingRate) error {
	notional := p.Size * fr.MarkPrice

	// Sign: positive rate ⇒ LONG pays SHORT.
	var amount float64
	if p.Side == "LONG" {
		amount = -fr.Rate * notional
	} else {
		amount = fr.Rate * notional
	}

	if amount == 0 {
		return nil
	}

	pay := &model.FundingPayment{
		PositionID:    p.ID,
		UserID:        p.UserID,
		FundingRateID: fr.ID,
		Pair:          p.Pair,
		Side:          p.Side,
		Notional:      notional,
		Rate:          fr.Rate,
		Amount:        amount,
	}
	if err := s.repo.CreatePayment(pay); err != nil {
		return err
	}

	// Apply to USDT wallet via gRPC. Negative amount = deduct.
	if amount > 0 {
		_ = s.walletClient.Credit(ctx, p.UserID, "USDT", amount)
	} else {
		_ = s.walletClient.Deduct(ctx, p.UserID, "USDT", -amount)
	}

	// Publish balance change event for projector / audit / UI.
	s.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
		UserID: p.UserID, Currency: "USDT",
		Delta:  amount,
		Reason: "funding",
		RefID:  fmt.Sprintf("funding-%d", fr.ID),
	})
	return nil
}

func (s *FundingService) markPrice(ctx context.Context, pair string) (float64, bool) {
	val, err := s.rdb.Get(ctx, "price:"+pair).Float64()
	if err != nil {
		return 0, false
	}
	return val, true
}

// indexPrice — placeholder. In production this is the spot index from
// multiple exchanges (volume-weighted). Here we use the same Redis key.
func (s *FundingService) indexPrice(ctx context.Context, pair string) (float64, bool) {
	val, err := s.rdb.Get(ctx, "index:"+pair).Float64()
	if err != nil {
		return s.markPrice(ctx, pair)
	}
	return val, true
}

// LatestRate returns the most recent funding rate for a pair (cached).
func (s *FundingService) LatestRate(pair string) (*model.FundingRate, error) {
	return s.repo.LatestRate(pair)
}

func (s *FundingService) RecentRates(pair string, limit int) ([]model.FundingRate, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	return s.repo.RecentRates(pair, limit)
}

func (s *FundingService) UserHistory(userID uint, page, size int) ([]model.FundingPayment, int64, error) {
	return s.repo.HistoryByUser(userID, page, size)
}

// nextFundingTime returns the next 00:00/08:00/16:00 UTC after `now`.
func nextFundingTime(now time.Time) time.Time {
	utc := now.UTC()
	hour := utc.Hour()
	var nextHour int
	switch {
	case hour < 8:
		nextHour = 8
	case hour < 16:
		nextHour = 16
	default:
		nextHour = 24 // tomorrow 00:00
	}
	t := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).
		Add(time.Duration(nextHour) * time.Hour)
	return t
}
