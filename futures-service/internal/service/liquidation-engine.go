package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	svcgrpc "github.com/cryptox/futures-service/internal/grpc"
	"github.com/cryptox/futures-service/internal/model"
	"github.com/cryptox/futures-service/internal/repository"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/ws"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type posWithPnL struct {
	pos       *model.FuturesPosition
	markPrice float64
	pnl       float64
}

// LiquidationEngine watches open positions and force-closes ones that breach
// the maintenance margin. Wallet credits go via gRPC — futures-service does NOT
// own the wallet table.
type LiquidationEngine struct {
	db           *gorm.DB
	positionRepo repository.PositionRepo
	walletClient svcgrpc.WalletClienter
	rdb          *redis.Client
	hub          *ws.Hub
	bus          eventbus.EventPublisher
}

func NewLiquidationEngine(
	positionRepo repository.PositionRepo,
	walletClient svcgrpc.WalletClienter,
	db *gorm.DB,
	rdb *redis.Client,
	hub *ws.Hub,
	bus eventbus.EventPublisher,
) *LiquidationEngine {
	return &LiquidationEngine{
		db:           db,
		positionRepo: positionRepo,
		walletClient: walletClient,
		rdb:          rdb,
		hub:          hub,
		bus:          bus,
	}
}

func (le *LiquidationEngine) getPrice(pair string) float64 {
	ctx := context.Background()
	price, err := le.rdb.Get(ctx, "price:"+pair).Float64()
	if err != nil {
		return 0
	}
	return price
}

// Start runs liquidation checks every 5 seconds
func (le *LiquidationEngine) Start() {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			le.check()
		}
	}()
}

func (le *LiquidationEngine) check() {
	positions, err := le.positionRepo.FindAllOpen()
	if err != nil {
		log.Printf("liquidation check error: %v", err)
		return
	}

	for i := range positions {
		pos := &positions[i]
		markPrice := le.getPrice(pos.Pair)
		if markPrice <= 0 {
			continue
		}

		if le.shouldTriggerTPSL(pos, markPrice) {
			le.autoClose(pos.ID, markPrice, "TP/SL")
			continue
		}

		shouldLiquidate := false
		if pos.Side == "LONG" && markPrice <= pos.LiquidationPrice {
			shouldLiquidate = true
		} else if pos.Side == "SHORT" && markPrice >= pos.LiquidationPrice {
			shouldLiquidate = true
		}
		if shouldLiquidate {
			le.liquidate(pos.ID, markPrice)
		}
	}

	le.checkAccountMargin(positions)
}

func (le *LiquidationEngine) checkAccountMargin(positions []model.FuturesPosition) {
	userPositions := make(map[uint][]posWithPnL)
	for i := range positions {
		pos := &positions[i]
		if pos.Status != "OPEN" {
			continue
		}
		markPrice := le.getPrice(pos.Pair)
		if markPrice <= 0 {
			continue
		}
		pnl := pos.CalcUnrealizedPnL(markPrice)
		userPositions[pos.UserID] = append(userPositions[pos.UserID], posWithPnL{
			pos: pos, markPrice: markPrice, pnl: pnl,
		})
	}

	for userID, posList := range userPositions {
		if len(posList) < 1 {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		balance, _, err := le.walletClient.GetBalance(ctx, userID, "USDT")
		cancel()
		if err != nil {
			continue
		}

		totalPositionValue := 0.0
		totalMaintenanceMargin := 0.0
		for _, p := range posList {
			posValue := p.pos.Margin + p.pnl
			if posValue < 0 {
				posValue = 0
			}
			totalPositionValue += posValue
			totalMaintenanceMargin += p.pos.Size * p.markPrice * 0.005
		}

		equity := balance + totalPositionValue
		if equity >= totalMaintenanceMargin {
			continue
		}

		log.Printf("MARGIN CALL user %d: equity=%.2f < maintenance=%.2f, positions=%d",
			userID, equity, totalMaintenanceMargin, len(posList))

		sortByPnL(posList)

		for _, p := range posList {
			if equity >= totalMaintenanceMargin {
				break
			}
			le.marginCallClose(p.pos.ID, p.markPrice, p.pnl)

			returnAmt := p.pos.Margin + p.pnl
			if returnAmt < 0 {
				returnAmt = 0
			}
			equity += returnAmt
			posMaintenanceReq := p.pos.Size * p.markPrice * 0.005
			totalMaintenanceMargin -= posMaintenanceReq
		}
	}
}

func sortByPnL(list []posWithPnL) {
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

func (le *LiquidationEngine) marginCallClose(positionID uint, markPrice, pnl float64) {
	ctx := context.Background()
	now := time.Now()
	var capturedPos *model.FuturesPosition
	var netPnL float64
	txErr := le.db.Transaction(func(tx *gorm.DB) error {
		pos, err := le.positionRepo.FindByIDForUpdate(tx, positionID, "OPEN")
		if err != nil {
			return err
		}

		// Cap loss at margin — user never owes from a forced close.
		netPnL = pnl
		if netPnL < -pos.Margin {
			netPnL = -pos.Margin
		}
		// Store realized PnL net of open fee so UI matches wallet impact.
		// Margin-call has no close fee (forced exit).
		openFee := pos.Size * pos.EntryPrice * 0.0005
		realizedPnL := netPnL - openFee
		pos.Status = "LIQUIDATED"
		pos.ClosedAt = &now
		pos.MarkPrice = markPrice
		pos.UnrealizedPnL = realizedPnL
		if err := le.positionRepo.Save(tx, pos); err != nil {
			return err
		}
		capturedPos = pos
		return nil
	})
	if txErr != nil {
		if !errors.Is(txErr, gorm.ErrRecordNotFound) {
			log.Printf("margin call tx error for position %d: %v", positionID, txErr)
		}
		return
	}
	if capturedPos == nil {
		return
	}
	pos := capturedPos

	// Settle wallet after DB commit. Failures here are logged + reconciled
	// via the balance.changed event consumer.
	_ = le.walletClient.Unlock(ctx, pos.UserID, "USDT", pos.Margin)
	if netPnL > 0 {
		if err := le.walletClient.Credit(ctx, pos.UserID, "USDT", netPnL); err != nil {
			log.Printf("[margin-call] wallet credit failed user=%d amt=%.4f: %v",
				pos.UserID, netPnL, err)
		}
	} else if netPnL < 0 {
		if err := le.walletClient.Deduct(ctx, pos.UserID, "USDT", -netPnL); err != nil {
			log.Printf("[margin-call] wallet deduct failed user=%d amt=%.4f: %v",
				pos.UserID, -netPnL, err)
		}
	}
	if le.bus != nil {
		le.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
			UserID: pos.UserID, Currency: "USDT",
			Delta: netPnL, Reason: "margin-call",
			RefID: fmt.Sprintf("position-%d", pos.ID),
		})
	}

	le.hub.Broadcast(fmt.Sprintf("liquidation@%d", pos.UserID), map[string]interface{}{
		"event": "MARGIN_CALL", "positionId": pos.ID, "pair": pos.Pair,
		"side": pos.Side, "pnl": pnl, "markPrice": markPrice,
	})
	if le.bus != nil {
		le.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
			UserID:  pos.UserID,
			Type:    "MARGIN_CALL",
			Title:   "Margin Call!",
			Message: fmt.Sprintf("%s %s force-closed, PnL: $%+.2f", pos.Side, pos.Pair, pnl),
			Pair:    pos.Pair,
		})
	}
	log.Printf("MARGIN CALL liquidated position %d: pair=%s side=%s pnl=%.4f",
		pos.ID, pos.Pair, pos.Side, pnl)
}

func (le *LiquidationEngine) shouldTriggerTPSL(pos *model.FuturesPosition, markPrice float64) bool {
	if pos.Side == "LONG" {
		if pos.TakeProfit > 0 && markPrice >= pos.TakeProfit {
			return true
		}
		if pos.StopLoss > 0 && markPrice <= pos.StopLoss {
			return true
		}
	} else {
		if pos.TakeProfit > 0 && markPrice <= pos.TakeProfit {
			return true
		}
		if pos.StopLoss > 0 && markPrice >= pos.StopLoss {
			return true
		}
	}
	return false
}

func (le *LiquidationEngine) autoClose(positionID uint, markPrice float64, reason string) {
	ctx := context.Background()
	now := time.Now()
	var capturedPos *model.FuturesPosition
	var pnl float64
	txErr := le.db.Transaction(func(tx *gorm.DB) error {
		pos, err := le.positionRepo.FindByIDForUpdate(tx, positionID, "OPEN")
		if err != nil {
			return err
		}

		pnl = pos.CalcUnrealizedPnL(markPrice)
		// Cap loss at margin (TP/SL never owe).
		if pnl < -pos.Margin {
			pnl = -pos.Margin
		}
		// TP/SL paths charge open fee but no close fee (force-close courtesy).
		// Store realized PnL net of open fee so UI matches wallet impact.
		openFee := pos.Size * pos.EntryPrice * 0.0005
		realizedPnL := pnl - openFee
		pos.Status = "CLOSED"
		pos.ClosedAt = &now
		pos.MarkPrice = markPrice
		pos.UnrealizedPnL = realizedPnL
		if err := le.positionRepo.Save(tx, pos); err != nil {
			return err
		}
		capturedPos = pos
		return nil
	})
	if txErr != nil {
		if !errors.Is(txErr, gorm.ErrRecordNotFound) {
			log.Printf("%s tx error for position %d: %v", reason, positionID, txErr)
		}
		return
	}
	if capturedPos == nil {
		return
	}
	pos := capturedPos

	// Settle wallet: unlock margin, then apply net PnL on available.
	_ = le.walletClient.Unlock(ctx, pos.UserID, "USDT", pos.Margin)
	if pnl > 0 {
		if err := le.walletClient.Credit(ctx, pos.UserID, "USDT", pnl); err != nil {
			log.Printf("[autoClose] wallet credit failed user=%d amt=%.4f: %v",
				pos.UserID, pnl, err)
		}
	} else if pnl < 0 {
		if err := le.walletClient.Deduct(ctx, pos.UserID, "USDT", -pnl); err != nil {
			log.Printf("[autoClose] wallet deduct failed user=%d amt=%.4f: %v",
				pos.UserID, -pnl, err)
		}
	}
	if le.bus != nil {
		le.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
			UserID: pos.UserID, Currency: "USDT",
			Delta: pnl, Reason: reason,
			RefID: fmt.Sprintf("position-%d", pos.ID),
		})
	}

	le.hub.Broadcast(fmt.Sprintf("position@%d", pos.UserID), map[string]interface{}{
		"event": reason, "positionId": pos.ID, "pair": pos.Pair,
		"side": pos.Side, "pnl": pnl, "markPrice": markPrice,
	})
	if le.bus != nil {
		le.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
			UserID:  pos.UserID,
			Type:    "POSITION_CLOSED",
			Title:   reason + " Triggered",
			Message: fmt.Sprintf("%s %s %s triggered at $%.2f, PnL: $%+.2f", pos.Side, pos.Pair, reason, markPrice, pnl),
			Pair:    pos.Pair,
		})
	}
	log.Printf("position %d %s triggered: pair=%s side=%s price=%.2f pnl=%.4f",
		pos.ID, reason, pos.Pair, pos.Side, markPrice, pnl)
}

func (le *LiquidationEngine) liquidate(positionID uint, markPrice float64) {
	ctx := context.Background()
	now := time.Now()
	var capturedPos *model.FuturesPosition
	var liquidated bool
	txErr := le.db.Transaction(func(tx *gorm.DB) error {
		pos, err := le.positionRepo.FindByIDForUpdate(tx, positionID, "OPEN")
		if err != nil {
			return err
		}

		currentPrice := le.getPrice(pos.Pair)
		if currentPrice <= 0 {
			currentPrice = markPrice
		}

		stillNeedsLiq := false
		if pos.Side == "LONG" && currentPrice <= pos.LiquidationPrice {
			stillNeedsLiq = true
		} else if pos.Side == "SHORT" && currentPrice >= pos.LiquidationPrice {
			stillNeedsLiq = true
		}
		if !stillNeedsLiq {
			return nil
		}

		pos.Status = "LIQUIDATED"
		pos.ClosedAt = &now
		pos.MarkPrice = currentPrice
		pos.UnrealizedPnL = -pos.Margin
		if err := le.positionRepo.Save(tx, pos); err != nil {
			return err
		}
		capturedPos = pos
		liquidated = true
		return nil
	})
	if txErr != nil {
		if !errors.Is(txErr, gorm.ErrRecordNotFound) {
			log.Printf("liquidation tx error for position %d: %v", positionID, txErr)
		}
		return
	}
	if !liquidated || capturedPos == nil {
		return
	}
	pos := capturedPos

	// Full margin loss: unlock then deduct = move margin from locked → gone.
	// Net effect: locked decreases by margin, available unchanged.
	_ = le.walletClient.Unlock(ctx, pos.UserID, "USDT", pos.Margin)
	if err := le.walletClient.Deduct(ctx, pos.UserID, "USDT", pos.Margin); err != nil {
		log.Printf("[liquidate] wallet deduct failed user=%d amt=%.4f: %v",
			pos.UserID, pos.Margin, err)
	}
	if le.bus != nil {
		le.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
			UserID: pos.UserID, Currency: "USDT",
			Delta: -pos.Margin, Reason: "liquidation",
			RefID: fmt.Sprintf("position-%d", pos.ID),
		})
	}

	le.hub.Broadcast(fmt.Sprintf("liquidation@%d", pos.UserID), map[string]interface{}{
		"positionId": pos.ID, "pair": pos.Pair, "side": pos.Side,
		"markPrice": pos.MarkPrice, "margin": pos.Margin,
	})
	if le.bus != nil {
		le.bus.Publish(ctx, eventbus.TopicNotificationCreated, eventbus.NotificationEvent{
			UserID:  pos.UserID,
			Type:    "POSITION_LIQUIDATED",
			Title:   "Position Liquidated!",
			Message: fmt.Sprintf("%s %s liquidated at $%.2f. Margin $%.2f lost.", pos.Side, pos.Pair, pos.MarkPrice, pos.Margin),
			Pair:    pos.Pair,
		})
	}
	log.Printf("position %d liquidated: pair=%s side=%s price=%.2f margin=%.2f",
		pos.ID, pos.Pair, pos.Side, pos.MarkPrice, pos.Margin)
}
