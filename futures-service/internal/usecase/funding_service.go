package usecase

import (
	"context"
	"log"
	"time"

	"github.com/cryptox/futures-service/internal/domain"
	"github.com/cryptox/shared/eventbus"
)

// FundingInterval is the settlement period (8h industry default).
const FundingInterval = 8 * time.Hour

// FundingService computes and settles perpetual funding every 8h.
type FundingService struct {
	funding   domain.FundingRepository
	positions domain.PositionRepository
	wallet    WalletGateway
	price     PriceSource
	rateCache RateCache
	bus       EventPublisher
}

func NewFundingService(funding domain.FundingRepository, positions domain.PositionRepository, wallet WalletGateway, price PriceSource, rateCache RateCache, bus EventPublisher) *FundingService {
	return &FundingService{funding: funding, positions: positions, wallet: wallet, price: price, rateCache: rateCache, bus: bus}
}

// Start ticks at each funding boundary and settles all pairs.
func (s *FundingService) Start(ctx context.Context, pairs []string) {
	go func() {
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

// Settle records a funding rate per pair and charges every open position.
func (s *FundingService) Settle(ctx context.Context, pairs []string, settledAt time.Time) error {
	for _, pair := range pairs {
		mark, ok := s.price.MarkPrice(ctx, pair)
		if !ok || mark <= 0 {
			continue
		}
		index, ok := s.price.IndexPrice(ctx, pair)
		if !ok || index <= 0 {
			index = mark
		}
		rate := domain.CalcFundingRate(mark, index)
		fr := &domain.FundingRate{
			Pair: pair, Rate: rate, IndexPrice: index, MarkPrice: mark,
			Interval: "8h", SettledAt: settledAt.UTC(), CreatedAt: time.Now(),
		}
		if err := s.funding.CreateRate(ctx, fr); err != nil {
			log.Printf("[funding] save rate %s: %v", pair, err)
			continue
		}
		s.rateCache.StoreLatestRate(ctx, pair, rate)

		positions, err := s.positions.FindAllOpen(ctx)
		if err != nil {
			log.Printf("[funding] load open: %v", err)
			continue
		}
		for i := range positions {
			if positions[i].Pair != pair {
				continue
			}
			if err := s.applyPayment(ctx, &positions[i], fr); err != nil {
				log.Printf("[funding] apply pos=%d: %v", positions[i].ID, err)
			}
		}
	}
	return nil
}

func (s *FundingService) applyPayment(ctx context.Context, p *domain.Position, fr *domain.FundingRate) error {
	notional := p.Notional(fr.MarkPrice)
	amount := domain.FundingAmount(p.Side, fr.Rate, notional)
	if amount == 0 {
		return nil
	}
	pay := &domain.FundingPayment{
		PositionID: p.ID, UserID: p.UserID, FundingRateID: fr.ID, Pair: p.Pair,
		Side: p.Side, Notional: notional, Rate: fr.Rate, Amount: amount,
	}
	if err := s.funding.CreatePayment(ctx, pay); err != nil {
		return err
	}
	if amount > 0 {
		_ = s.wallet.Credit(ctx, p.UserID, usdt, amount)
	} else {
		_ = s.wallet.Deduct(ctx, p.UserID, usdt, -amount)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
			UserID: p.UserID, Currency: usdt, Delta: amount, Reason: "funding",
			RefID: "funding-" + itoa(fr.ID),
		})
	}
	return nil
}

// Queries
func (s *FundingService) LatestRate(ctx context.Context, pair string) (*domain.FundingRate, error) {
	return s.funding.LatestRate(ctx, pair)
}

func (s *FundingService) RecentRates(ctx context.Context, pair string, limit int) ([]domain.FundingRate, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	return s.funding.RecentRates(ctx, pair, limit)
}

func (s *FundingService) UserHistory(ctx context.Context, userID uint, page, size int) ([]domain.FundingPayment, int64, error) {
	return s.funding.HistoryByUser(ctx, userID, page, size)
}

// nextFundingTime returns the next 00:00 / 08:00 / 16:00 UTC after now.
func nextFundingTime(now time.Time) time.Time {
	utc := now.UTC()
	var nextHour int
	switch {
	case utc.Hour() < 8:
		nextHour = 8
	case utc.Hour() < 16:
		nextHour = 16
	default:
		nextHour = 24
	}
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(nextHour) * time.Hour)
}

func itoa(u uint) string {
	if u == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for u > 0 {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
	}
	return string(b[i:])
}
