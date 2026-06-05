package postgres

import (
	"context"
	"fmt"

	"github.com/cryptox/trading-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/trading-service/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TradeRepo is the pgx+sqlc adapter for domain.TradeRepository.
type TradeRepo struct{ q *sqlc.Queries }

func NewTradeRepo(pool *pgxpool.Pool) *TradeRepo { return &TradeRepo{q: sqlc.New(pool)} }

var _ domain.TradeRepository = (*TradeRepo)(nil)

func (r *TradeRepo) Create(ctx context.Context, t *domain.Trade) error {
	row, err := r.q.CreateTrade(ctx, sqlc.CreateTradeParams{
		Pair: t.Pair, BuyOrderID: int64(t.BuyOrderID), SellOrderID: int64(t.SellOrderID),
		BuyerID: int64(t.BuyerID), SellerID: int64(t.SellerID), Price: t.Price, Amount: t.Amount,
		Total: t.Total, BuyerFee: t.BuyerFee, SellerFee: t.SellerFee,
	})
	if err != nil {
		return fmt.Errorf("postgres: create trade: %w", err)
	}
	t.ID = uint(row.ID)
	t.CreatedAt = row.CreatedAt
	return nil
}
