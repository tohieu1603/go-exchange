package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/cryptox/futures-service/internal/domain"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/metrics"
)

// OpenCommand is the input to opening a position.
type OpenCommand struct {
	Pair       string
	Side       string
	Leverage   int
	Size       float64
	TakeProfit float64
	StopLoss   float64
}

// PositionService is the futures command/query use case for end users.
type PositionService struct {
	positions   domain.PositionRepository
	wallet      WalletGateway
	price       PriceSource
	tx          TxManager
	broadcaster Broadcaster
	bus         EventPublisher
}

func NewPositionService(positions domain.PositionRepository, wallet WalletGateway, price PriceSource, tx TxManager, b Broadcaster, bus EventPublisher) *PositionService {
	return &PositionService{positions: positions, wallet: wallet, price: price, tx: tx, broadcaster: b, bus: bus}
}

// OpenPosition locks margin + charges the open fee, then records the position.
// Every wallet step is compensated on a later failure so user-visible balances
// never drift.
func (s *PositionService) OpenPosition(ctx context.Context, userID uint, cmd OpenCommand) (*domain.Position, error) {
	mark, ok := s.price.MarkPrice(ctx, cmd.Pair)
	if !ok || mark <= 0 {
		return nil, domain.ErrPriceUnavailable
	}
	pos, err := domain.OpenPosition(userID, cmd.Pair, cmd.Side, cmd.Leverage, cmd.Size, mark, cmd.TakeProfit, cmd.StopLoss)
	if err != nil {
		return nil, err
	}
	openFee := pos.OpenFee()

	if err := s.wallet.CheckBalance(ctx, userID, usdt, pos.Margin+openFee); err != nil {
		return nil, err
	}
	if err := s.wallet.Lock(ctx, userID, usdt, pos.Margin); err != nil {
		return nil, err
	}
	if err := s.wallet.Deduct(ctx, userID, usdt, openFee); err != nil {
		_ = s.wallet.Unlock(ctx, userID, usdt, pos.Margin)
		return nil, err
	}
	if err := s.positions.Create(ctx, pos); err != nil {
		_ = s.wallet.Credit(ctx, userID, usdt, openFee)
		_ = s.wallet.Unlock(ctx, userID, usdt, pos.Margin)
		return nil, err
	}
	metrics.FuturesOpenPositions.Inc()

	s.broadcaster.Broadcast(fmt.Sprintf("position@%d", userID), pos)
	s.publishOpened(ctx, pos)
	return pos, nil
}

// ClosePosition closes a user's position under a row lock and settles the wallet.
func (s *PositionService) ClosePosition(ctx context.Context, userID, positionID uint) (*domain.Position, error) {
	var pos *domain.Position
	var settle domain.Settlement
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		p, err := s.positions.FindByUserAndIDForUpdate(ctx, userID, positionID, domain.StatusOpen)
		if err != nil {
			return err
		}
		mark, ok := s.price.MarkPrice(ctx, p.Pair)
		if !ok || mark <= 0 {
			return domain.ErrPriceUnavailable
		}
		settle = p.Close(mark, time.Now())
		pos = p
		return s.positions.Save(ctx, p)
	})
	if err != nil {
		return nil, err
	}
	metrics.FuturesOpenPositions.Dec()

	settleWallet(ctx, s.wallet, pos.UserID, settle)
	s.broadcaster.Broadcast(fmt.Sprintf("position@%d", userID), pos)
	s.publishClosed(ctx, pos)
	return pos, nil
}

// UpdateTPSL updates take-profit / stop-loss on an open position.
func (s *PositionService) UpdateTPSL(ctx context.Context, userID, positionID uint, tp, sl *float64) (*domain.Position, error) {
	pos, err := s.positions.FindByUserAndID(ctx, userID, positionID, domain.StatusOpen)
	if err != nil {
		return nil, domain.ErrPositionNotFound
	}
	if tp == nil && sl == nil {
		return pos, nil
	}
	if err := s.positions.UpdateTPSL(ctx, positionID, userID, tp, sl); err != nil {
		return nil, err
	}
	if tp != nil {
		pos.TakeProfit = *tp
	}
	if sl != nil {
		pos.StopLoss = *sl
	}
	return pos, nil
}

// GetPositions returns a user's positions (optionally filtered), with live PnL
// for open ones.
func (s *PositionService) GetPositions(ctx context.Context, userID uint, status string) ([]domain.Position, error) {
	positions, err := s.positions.FindByUserAndStatus(ctx, userID, status)
	if err != nil {
		return nil, err
	}
	s.enrichLivePnL(ctx, positions)
	return positions, nil
}

func (s *PositionService) GetOpenPositions(ctx context.Context, userID uint) ([]domain.Position, error) {
	positions, err := s.positions.FindOpenByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.enrichLivePnL(ctx, positions)
	return positions, nil
}

func (s *PositionService) enrichLivePnL(ctx context.Context, positions []domain.Position) {
	for i := range positions {
		if positions[i].Status != domain.StatusOpen {
			continue
		}
		if mp, ok := s.price.MarkPrice(ctx, positions[i].Pair); ok && mp > 0 {
			positions[i].MarkPrice = mp
			positions[i].UnrealizedPnL = positions[i].CalcUnrealizedPnL(mp)
		}
	}
}

func (s *PositionService) publishOpened(ctx context.Context, pos *domain.Position) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
		UserID: pos.UserID, Type: "POSITION_OPENED", Title: "Position Opened",
		Message: fmt.Sprintf("%s %s %dx %.4f @ $%.2f", pos.Side, pos.Pair, pos.Leverage, pos.Size, pos.EntryPrice),
		Pair:    pos.Pair,
	})
	s.bus.Publish(ctx, eventbus.TopicPositionChanged, positionEvent(pos))
}

func (s *PositionService) publishClosed(ctx context.Context, pos *domain.Position) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
		UserID: pos.UserID, Type: "POSITION_CLOSED", Title: "Position Closed",
		Message: fmt.Sprintf("%s %s closed, PnL: $%+.2f", pos.Side, pos.Pair, pos.UnrealizedPnL),
		Pair:    pos.Pair,
	})
	s.bus.Publish(ctx, eventbus.TopicPositionChanged, positionEvent(pos))
}

func positionEvent(p *domain.Position) eventbus.PositionEvent {
	return eventbus.PositionEvent{
		PositionID: p.ID, UserID: p.UserID, Pair: p.Pair, Side: p.Side, Status: p.Status,
		EntryPrice: p.EntryPrice, MarkPrice: p.MarkPrice, Size: p.Size, Margin: p.Margin, PnL: p.UnrealizedPnL,
	}
}
