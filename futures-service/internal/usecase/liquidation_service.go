package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/futures-service/internal/domain"
	"github.com/cryptox/shared/eventbus"
)

// settleWallet unlocks margin then applies the signed net delta — shared by the
// position and liquidation use cases.
func settleWallet(ctx context.Context, wallet WalletGateway, userID uint, st domain.Settlement) {
	_ = wallet.Unlock(ctx, userID, usdt, st.UnlockMargin)
	if st.NetDelta > 0 {
		_ = wallet.Credit(ctx, userID, usdt, st.NetDelta)
	} else if st.NetDelta < 0 {
		_ = wallet.Deduct(ctx, userID, usdt, -st.NetDelta)
	}
}

// LiquidationService periodically force-closes positions that breach TP/SL, the
// liquidation price, or the portfolio maintenance margin.
type LiquidationService struct {
	positions   domain.PositionRepository
	wallet      WalletGateway
	price       PriceSource
	tx          TxManager
	broadcaster Broadcaster
	bus         EventPublisher
	interval    time.Duration
}

func NewLiquidationService(positions domain.PositionRepository, wallet WalletGateway, price PriceSource, tx TxManager, b Broadcaster, bus EventPublisher) *LiquidationService {
	return &LiquidationService{positions: positions, wallet: wallet, price: price, tx: tx, broadcaster: b, bus: bus, interval: 5 * time.Second}
}

// Start runs liquidation sweeps until ctx is cancelled.
func (le *LiquidationService) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(le.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				le.Check(ctx)
			}
		}
	}()
}

// Check sweeps all open positions once.
func (le *LiquidationService) Check(ctx context.Context) {
	positions, err := le.positions.FindAllOpen(ctx)
	if err != nil {
		log.Printf("[liquidation] load open: %v", err)
		return
	}
	for i := range positions {
		pos := &positions[i]
		mark, ok := le.price.MarkPrice(ctx, pos.Pair)
		if !ok || mark <= 0 {
			continue
		}
		if pos.ShouldTriggerTPSL(mark) {
			le.autoClose(ctx, pos.ID, mark, "TP/SL")
			continue
		}
		if pos.ShouldLiquidate(mark) {
			le.liquidate(ctx, pos.ID, mark)
		}
	}
	le.checkAccountMargin(ctx, positions)
}

func (le *LiquidationService) autoClose(ctx context.Context, positionID uint, mark float64, reason string) {
	var pos *domain.Position
	var st domain.Settlement
	err := le.tx.Do(ctx, func(ctx context.Context) error {
		p, err := le.positions.FindByIDForUpdate(ctx, positionID, domain.StatusOpen)
		if err != nil {
			return err
		}
		st = p.ForceCloseTPSL(mark, time.Now())
		pos = p
		return le.positions.Save(ctx, p)
	})
	if err != nil {
		le.logTxErr(reason, positionID, err)
		return
	}
	settleWallet(ctx, le.wallet, pos.UserID, st)
	le.publishBalance(ctx, pos, st.NetDelta, reason)
	le.broadcaster.Broadcast(fmt.Sprintf("position@%d", pos.UserID), map[string]interface{}{
		"event": reason, "positionId": pos.ID, "pair": pos.Pair, "side": pos.Side, "pnl": st.NetDelta, "markPrice": mark,
	})
	le.notify(ctx, pos.UserID, "POSITION_CLOSED", reason+" Triggered",
		fmt.Sprintf("%s %s %s triggered at $%.2f, PnL: $%+.2f", pos.Side, pos.Pair, reason, mark, st.NetDelta), pos.Pair)
}

func (le *LiquidationService) liquidate(ctx context.Context, positionID uint, mark float64) {
	var pos *domain.Position
	var st domain.Settlement
	var done bool
	err := le.tx.Do(ctx, func(ctx context.Context) error {
		p, err := le.positions.FindByIDForUpdate(ctx, positionID, domain.StatusOpen)
		if err != nil {
			return err
		}
		// Re-check against the freshest price under the lock.
		cur := mark
		if m, ok := le.price.MarkPrice(ctx, p.Pair); ok && m > 0 {
			cur = m
		}
		if !p.ShouldLiquidate(cur) {
			return nil // price recovered — abort liquidation
		}
		st = p.Liquidate(cur, time.Now())
		pos = p
		done = true
		return le.positions.Save(ctx, p)
	})
	if err != nil {
		le.logTxErr("liquidation", positionID, err)
		return
	}
	if !done || pos == nil {
		return
	}
	settleWallet(ctx, le.wallet, pos.UserID, st)
	le.publishBalance(ctx, pos, st.NetDelta, "liquidation")
	le.broadcaster.Broadcast(fmt.Sprintf("liquidation@%d", pos.UserID), map[string]interface{}{
		"positionId": pos.ID, "pair": pos.Pair, "side": pos.Side, "markPrice": pos.MarkPrice, "margin": pos.Margin,
	})
	le.notify(ctx, pos.UserID, "POSITION_LIQUIDATED", "Position Liquidated!",
		fmt.Sprintf("%s %s liquidated at $%.2f. Margin $%.2f lost.", pos.Side, pos.Pair, pos.MarkPrice, pos.Margin), pos.Pair)
}

type posWithPnL struct {
	pos  *domain.Position
	mark float64
	pnl  float64
}

func (le *LiquidationService) checkAccountMargin(ctx context.Context, positions []domain.Position) {
	byUser := make(map[uint][]posWithPnL)
	for i := range positions {
		pos := &positions[i]
		if pos.Status != domain.StatusOpen {
			continue
		}
		mark, ok := le.price.MarkPrice(ctx, pos.Pair)
		if !ok || mark <= 0 {
			continue
		}
		byUser[pos.UserID] = append(byUser[pos.UserID], posWithPnL{pos: pos, mark: mark, pnl: pos.CalcUnrealizedPnL(mark)})
	}

	for userID, list := range byUser {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		balance, _, err := le.wallet.GetBalance(cctx, userID, usdt)
		cancel()
		if err != nil {
			continue
		}
		positionValue, maintenance := 0.0, 0.0
		for _, p := range list {
			v := p.pos.Margin + p.pnl
			if v < 0 {
				v = 0
			}
			positionValue += v
			maintenance += p.pos.MaintenanceMargin(p.mark)
		}
		equity := balance + positionValue
		if equity >= maintenance {
			continue
		}
		log.Printf("[margin-call] user=%d equity=%.2f < maintenance=%.2f positions=%d", userID, equity, maintenance, len(list))
		sortByPnLAsc(list)
		for _, p := range list {
			if equity >= maintenance {
				break
			}
			le.marginCallClose(ctx, p.pos.ID, p.mark, p.pnl)
			ret := p.pos.Margin + p.pnl
			if ret < 0 {
				ret = 0
			}
			equity += ret
			maintenance -= p.pos.MaintenanceMargin(p.mark)
		}
	}
}

func (le *LiquidationService) marginCallClose(ctx context.Context, positionID uint, mark, pnl float64) {
	var pos *domain.Position
	var st domain.Settlement
	err := le.tx.Do(ctx, func(ctx context.Context) error {
		p, err := le.positions.FindByIDForUpdate(ctx, positionID, domain.StatusOpen)
		if err != nil {
			return err
		}
		st = p.MarginCallClose(mark, pnl, time.Now())
		pos = p
		return le.positions.Save(ctx, p)
	})
	if err != nil {
		le.logTxErr("margin-call", positionID, err)
		return
	}
	settleWallet(ctx, le.wallet, pos.UserID, st)
	le.publishBalance(ctx, pos, st.NetDelta, "margin-call")
	le.broadcaster.Broadcast(fmt.Sprintf("liquidation@%d", pos.UserID), map[string]interface{}{
		"event": "MARGIN_CALL", "positionId": pos.ID, "pair": pos.Pair, "side": pos.Side, "pnl": st.NetDelta, "markPrice": mark,
	})
	le.notify(ctx, pos.UserID, "MARGIN_CALL", "Margin Call!",
		fmt.Sprintf("%s %s force-closed, PnL: $%+.2f", pos.Side, pos.Pair, st.NetDelta), pos.Pair)
}

func (le *LiquidationService) publishBalance(ctx context.Context, pos *domain.Position, delta float64, reason string) {
	if le.bus == nil {
		return
	}
	le.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
		UserID: pos.UserID, Currency: usdt, Delta: delta, Reason: reason, RefID: fmt.Sprintf("position-%d", pos.ID),
	})
}

func (le *LiquidationService) notify(ctx context.Context, userID uint, typ, title, msg, pair string) {
	if le.bus == nil {
		return
	}
	le.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
		UserID: userID, Type: typ, Title: title, Message: msg, Pair: pair,
	})
}

func (le *LiquidationService) logTxErr(reason string, id uint, err error) {
	if !errors.Is(err, domain.ErrPositionNotFound) {
		log.Printf("[%s] tx error for position %d: %v", reason, id, err)
	}
}

// sortByPnLAsc insertion-sorts positions by PnL ascending (worst first).
func sortByPnLAsc(list []posWithPnL) {
	for i := 1; i < len(list); i++ {
		key := list[i]
		j := i - 1
		for j >= 0 && list[j].pnl > key.pnl {
			list[j+1] = list[j]
			j--
		}
		list[j+1] = key
	}
}
