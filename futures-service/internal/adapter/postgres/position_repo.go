package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/cryptox/futures-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/futures-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PositionRepo is the pgx+sqlc adapter for domain.PositionRepository.
type PositionRepo struct{ pool *pgxpool.Pool }

func NewPositionRepo(pool *pgxpool.Pool) *PositionRepo { return &PositionRepo{pool: pool} }

var _ domain.PositionRepository = (*PositionRepo)(nil)

func (r *PositionRepo) Create(ctx context.Context, pos *domain.Position) error {
	row, err := q(ctx, r.pool).CreatePosition(ctx, sqlc.CreatePositionParams{
		UserID: int64(pos.UserID), Pair: pos.Pair, Side: pos.Side, Leverage: int32(pos.Leverage),
		EntryPrice: pos.EntryPrice, MarkPrice: pos.MarkPrice, Size: pos.Size, Margin: pos.Margin,
		UnrealizedPnL: pos.UnrealizedPnL, LiquidationPrice: pos.LiquidationPrice,
		TakeProfit: pos.TakeProfit, StopLoss: pos.StopLoss, Status: pos.Status,
	})
	if err != nil {
		return fmt.Errorf("postgres: create position: %w", err)
	}
	pos.ID = uint(row.ID)
	pos.CreatedAt = row.CreatedAt
	return nil
}

func (r *PositionRepo) FindOpenByUser(ctx context.Context, userID uint) ([]domain.Position, error) {
	rows, err := q(ctx, r.pool).FindOpenPositionsByUser(ctx, int64(userID))
	if err != nil {
		return nil, fmt.Errorf("postgres: find open positions by user: %w", err)
	}
	return positionsToDomain(rows), nil
}

func (r *PositionRepo) FindByUserAndStatus(ctx context.Context, userID uint, status string) ([]domain.Position, error) {
	rows, err := q(ctx, r.pool).FindPositionsByUserAndStatus(ctx, sqlc.FindPositionsByUserAndStatusParams{UserID: int64(userID), Status: status})
	if err != nil {
		return nil, fmt.Errorf("postgres: find positions by user+status: %w", err)
	}
	return positionsToDomain(rows), nil
}

func (r *PositionRepo) FindAllOpen(ctx context.Context) ([]domain.Position, error) {
	rows, err := q(ctx, r.pool).FindAllOpenPositions(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: find all open positions: %w", err)
	}
	return positionsToDomain(rows), nil
}

func (r *PositionRepo) FindByIDForUpdate(ctx context.Context, id uint, status string) (*domain.Position, error) {
	row, err := q(ctx, r.pool).FindPositionByIDForUpdate(ctx, sqlc.FindPositionByIDForUpdateParams{ID: int64(id), Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPositionNotFound
		}
		return nil, fmt.Errorf("postgres: find position by id for update: %w", err)
	}
	p := positionToDomain(row)
	return &p, nil
}

func (r *PositionRepo) FindByUserAndIDForUpdate(ctx context.Context, userID, id uint, status string) (*domain.Position, error) {
	row, err := q(ctx, r.pool).FindPositionByUserAndIDForUpdate(ctx, sqlc.FindPositionByUserAndIDForUpdateParams{ID: int64(id), UserID: int64(userID), Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPositionNotFound
		}
		return nil, fmt.Errorf("postgres: find position by user+id for update: %w", err)
	}
	p := positionToDomain(row)
	return &p, nil
}

func (r *PositionRepo) FindByUserAndID(ctx context.Context, userID, id uint, status string) (*domain.Position, error) {
	row, err := q(ctx, r.pool).FindPositionByUserAndID(ctx, sqlc.FindPositionByUserAndIDParams{ID: int64(id), UserID: int64(userID), Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPositionNotFound
		}
		return nil, fmt.Errorf("postgres: find position by user+id: %w", err)
	}
	p := positionToDomain(row)
	return &p, nil
}

func (r *PositionRepo) Save(ctx context.Context, pos *domain.Position) error {
	if err := q(ctx, r.pool).SavePosition(ctx, sqlc.SavePositionParams{
		ID: int64(pos.ID), Pair: pos.Pair, Side: pos.Side, Leverage: int32(pos.Leverage),
		EntryPrice: pos.EntryPrice, MarkPrice: pos.MarkPrice, Size: pos.Size, Margin: pos.Margin,
		UnrealizedPnL: pos.UnrealizedPnL, LiquidationPrice: pos.LiquidationPrice,
		TakeProfit: pos.TakeProfit, StopLoss: pos.StopLoss, Status: pos.Status, ClosedAt: pos.ClosedAt,
	}); err != nil {
		return fmt.Errorf("postgres: save position: %w", err)
	}
	return nil
}

func (r *PositionRepo) UpdateTPSL(ctx context.Context, id, userID uint, takeProfit, stopLoss *float64) error {
	if err := q(ctx, r.pool).UpdateTPSL(ctx, sqlc.UpdateTPSLParams{
		ID: int64(id), UserID: int64(userID),
		TakeProfit: numericFromPtr(takeProfit), StopLoss: numericFromPtr(stopLoss),
	}); err != nil {
		return fmt.Errorf("postgres: update tp/sl: %w", err)
	}
	return nil
}

// numericFromPtr builds a pgtype.Numeric from an optional float; a nil pointer
// yields a NULL value so COALESCE leaves the column unchanged.
func numericFromPtr(f *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if f == nil {
		return n // Valid=false → NULL
	}
	_ = n.Scan(strconv.FormatFloat(*f, 'f', -1, 64))
	return n
}

func positionToDomain(m sqlc.FuturesPosition) domain.Position {
	return domain.Position{
		ID: uint(m.ID), UserID: uint(m.UserID), Pair: m.Pair, Side: m.Side, Leverage: int(m.Leverage),
		EntryPrice: m.EntryPrice, MarkPrice: m.MarkPrice, Size: m.Size, Margin: m.Margin,
		UnrealizedPnL: m.UnrealizedPnL, LiquidationPrice: m.LiquidationPrice,
		TakeProfit: m.TakeProfit, StopLoss: m.StopLoss, Status: m.Status,
		CreatedAt: m.CreatedAt, ClosedAt: m.ClosedAt,
	}
}

func positionsToDomain(ms []sqlc.FuturesPosition) []domain.Position {
	out := make([]domain.Position, len(ms))
	for i, m := range ms {
		out[i] = positionToDomain(m)
	}
	return out
}
