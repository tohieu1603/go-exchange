package postgres

import (
	"context"
	"fmt"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CoinRepo seeds the auth-owned coin catalog reference table. Coins are not read
// by any use case here — auth simply owns and seeds the shared `coins` table.
type CoinRepo struct{ q *sqlc.Queries }

func NewCoinRepo(pool *pgxpool.Pool) *CoinRepo { return &CoinRepo{q: sqlc.New(pool)} }

// SeedDefaults inserts the default coin list once (idempotent on symbol).
func (r *CoinRepo) SeedDefaults(ctx context.Context) error {
	n, err := r.q.CountCoins(ctx)
	if err != nil {
		return fmt.Errorf("postgres: count coins: %w", err)
	}
	if n > 0 {
		return nil
	}
	for _, c := range types.DefaultCoins {
		if err := r.q.InsertCoin(ctx, sqlc.InsertCoinParams{
			Symbol: c.Symbol, Name: c.Name, CoinGeckoID: c.CoinGeckoID, BybitSymbol: c.BybitSymbol,
			IconUrl: c.IconURL, IsActive: c.IsActive, SortOrder: int32(c.SortOrder), AssetType: c.AssetType,
		}); err != nil {
			return fmt.Errorf("postgres: seed coin %s: %w", c.Symbol, err)
		}
	}
	return nil
}
