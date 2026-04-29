package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	svcgrpc "github.com/cryptox/futures-service/internal/grpc"
	"github.com/cryptox/futures-service/internal/model"
	"github.com/cryptox/futures-service/internal/repository"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/ws"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type FuturesService struct {
	db           *gorm.DB
	positionRepo repository.PositionRepo
	walletClient *svcgrpc.WalletClient
	rdb          *redis.Client
	hub          *ws.Hub
	bus          eventbus.EventPublisher
}

func NewFuturesService(
	positionRepo repository.PositionRepo,
	walletClient *svcgrpc.WalletClient,
	db *gorm.DB,
	rdb *redis.Client,
	hub *ws.Hub,
	bus eventbus.EventPublisher,
) *FuturesService {
	return &FuturesService{
		db:           db,
		positionRepo: positionRepo,
		walletClient: walletClient,
		rdb:          rdb,
		hub:          hub,
		bus:          bus,
	}
}

type OpenPositionReq struct {
	Pair       string  `json:"pair" binding:"required"`
	Side       string  `json:"side" binding:"required"`
	Leverage   int     `json:"leverage" binding:"required,min=1,max=125"`
	Size       float64 `json:"size" binding:"required,gt=0"`
	TakeProfit float64 `json:"takeProfit"`
	StopLoss   float64 `json:"stopLoss"`
}

func (s *FuturesService) getPrice(pair string) float64 {
	ctx := context.Background()
	price, err := s.rdb.Get(ctx, "price:"+pair).Float64()
	if err != nil {
		return 0
	}
	return price
}

// OpenPosition: Lock(margin) + Deduct(fee) → DB create position.
// Margin is locked (not deducted) so the user's `locked` balance reflects the
// collateral — total wallet equity stays visible as available + locked.
// Only the trading fee is a real expense.
func (s *FuturesService) OpenPosition(userID uint, req OpenPositionReq) (*model.FuturesPosition, error) {
	ctx := context.Background()

	markPrice := s.getPrice(req.Pair)
	if markPrice <= 0 {
		return nil, errors.New("price unavailable")
	}

	notional := req.Size * markPrice
	margin := notional / float64(req.Leverage)
	fee := notional * 0.0005
	totalCost := margin + fee
	liqPrice := model.CalcLiquidationPrice(req.Side, markPrice, req.Leverage)

	if err := s.walletClient.CheckBalance(ctx, userID, "USDT", totalCost); err != nil {
		return nil, err
	}
	if err := s.walletClient.Lock(ctx, userID, "USDT", margin); err != nil {
		return nil, err
	}
	if err := s.walletClient.Deduct(ctx, userID, "USDT", fee); err != nil {
		// Roll back the lock — user-visible state must stay consistent.
		_ = s.walletClient.Unlock(ctx, userID, "USDT", margin)
		return nil, err
	}

	pos := &model.FuturesPosition{
		UserID: userID, Pair: req.Pair, Side: req.Side,
		Leverage: req.Leverage, EntryPrice: markPrice, MarkPrice: markPrice,
		Size: req.Size, Margin: margin, LiquidationPrice: liqPrice,
		TakeProfit: req.TakeProfit, StopLoss: req.StopLoss, Status: "OPEN",
	}
	if err := s.positionRepo.Create(nil, pos); err != nil {
		// Best-effort refund on DB failure: refund fee + unlock margin.
		_ = s.walletClient.Credit(ctx, userID, "USDT", fee)
		_ = s.walletClient.Unlock(ctx, userID, "USDT", margin)
		return nil, err
	}
	metrics.FuturesOpenPositions.Inc()

	s.hub.Broadcast(fmt.Sprintf("position@%d", userID), pos)

	if s.bus != nil {
		s.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
			UserID:  userID,
			Type:    "POSITION_OPENED",
			Title:   "Position Opened",
			Message: fmt.Sprintf("%s %s %dx %.4f @ $%.2f", req.Side, req.Pair, req.Leverage, req.Size, markPrice),
			Pair:    req.Pair,
		})
		s.bus.Publish(ctx, eventbus.TopicPositionChanged, eventbus.PositionEvent{
			PositionID: pos.ID, UserID: userID, Pair: req.Pair,
			Side: req.Side, Status: "OPEN", EntryPrice: markPrice,
			MarkPrice: markPrice, Size: req.Size, Margin: margin, PnL: 0,
		})
	}

	return pos, nil
}

// ClosePosition: DB SELECT FOR UPDATE → Unlock(margin) + Credit/Deduct net PnL.
// Net wallet effect after Unlock: balance changes by (pnl - fee).
// If the loss exceeds margin (rare, race vs liquidator), cap loss at margin.
func (s *FuturesService) ClosePosition(userID, positionID uint) (*model.FuturesPosition, error) {
	ctx := context.Background()
	var pos *model.FuturesPosition
	var pnl, fee float64

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		pos, err = s.positionRepo.FindByUserAndIDForUpdate(tx, userID, positionID, "OPEN")
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("position not found or already closed")
			}
			return err
		}

		markPrice := s.getPrice(pos.Pair)
		if markPrice <= 0 {
			return errors.New("price unavailable")
		}

		pnl = pos.CalcUnrealizedPnL(markPrice)
		fee = pos.Size * markPrice * 0.0005
		// Reconstruct open fee (rate is constant) so the realized PnL stored
		// on the position reflects what the user actually gained/lost net of
		// every trading cost — making the closed-position PnL column match
		// the wallet movement.
		openFee := pos.Size * pos.EntryPrice * 0.0005
		realizedPnL := pnl - fee - openFee

		now := time.Now()
		pos.Status = "CLOSED"
		pos.ClosedAt = &now
		pos.MarkPrice = markPrice
		pos.UnrealizedPnL = realizedPnL
		return s.positionRepo.Save(tx, pos)
	})
	if txErr != nil {
		return nil, txErr
	}
	metrics.FuturesOpenPositions.Dec()

	// Settle wallet: unlock margin, then apply net PnL & fee on available.
	// Cap net loss at margin so the user can never go negative from a close.
	netDelta := pnl - fee
	if netDelta < -pos.Margin {
		netDelta = -pos.Margin
	}
	_ = s.walletClient.Unlock(ctx, userID, "USDT", pos.Margin)
	if netDelta > 0 {
		_ = s.walletClient.Credit(ctx, userID, "USDT", netDelta)
	} else if netDelta < 0 {
		_ = s.walletClient.Deduct(ctx, userID, "USDT", -netDelta)
	}

	s.hub.Broadcast(fmt.Sprintf("position@%d", userID), pos)
	pnlStr := fmt.Sprintf("%+.2f", pos.UnrealizedPnL)

	if s.bus != nil {
		s.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
			UserID:  userID,
			Type:    "POSITION_CLOSED",
			Title:   "Position Closed",
			Message: fmt.Sprintf("%s %s closed, PnL: $%s", pos.Side, pos.Pair, pnlStr),
			Pair:    pos.Pair,
		})
		s.bus.Publish(ctx, eventbus.TopicPositionChanged, eventbus.PositionEvent{
			PositionID: pos.ID, UserID: userID, Pair: pos.Pair,
			Side: pos.Side, Status: "CLOSED", EntryPrice: pos.EntryPrice,
			MarkPrice: pos.MarkPrice, Size: pos.Size, Margin: pos.Margin, PnL: pos.UnrealizedPnL,
		})
	}

	return pos, nil
}

// UpdateTPSL updates take profit / stop loss on an open position
func (s *FuturesService) UpdateTPSL(userID, positionID uint, tp, sl *float64) (*model.FuturesPosition, error) {
	pos, err := s.positionRepo.FindByUserAndID(userID, positionID, "OPEN")
	if err != nil {
		return nil, errors.New("position not found or already closed")
	}

	updates := map[string]interface{}{}
	if tp != nil {
		updates["take_profit"] = *tp
		pos.TakeProfit = *tp
	}
	if sl != nil {
		updates["stop_loss"] = *sl
		pos.StopLoss = *sl
	}
	if len(updates) == 0 {
		return pos, nil
	}
	if err := s.positionRepo.UpdateTPSL(positionID, userID, updates); err != nil {
		return nil, err
	}
	return pos, nil
}

// GetPositions returns user's positions with optional status filter
func (s *FuturesService) GetPositions(userID uint, status string) ([]model.FuturesPosition, error) {
	positions, err := s.positionRepo.FindByUserAndStatus(userID, status)
	if err != nil {
		return nil, err
	}
	for i := range positions {
		if positions[i].Status == "OPEN" {
			mp := s.getPrice(positions[i].Pair)
			if mp > 0 {
				positions[i].MarkPrice = mp
				positions[i].UnrealizedPnL = positions[i].CalcUnrealizedPnL(mp)
			}
		}
	}
	return positions, nil
}

// GetOpenPositions returns only open positions with live PnL
func (s *FuturesService) GetOpenPositions(userID uint) ([]model.FuturesPosition, error) {
	positions, err := s.positionRepo.FindOpenByUser(userID)
	if err != nil {
		return nil, err
	}
	for i := range positions {
		mp := s.getPrice(positions[i].Pair)
		if mp > 0 {
			positions[i].MarkPrice = mp
			positions[i].UnrealizedPnL = positions[i].CalcUnrealizedPnL(mp)
		}
	}
	return positions, nil
}
