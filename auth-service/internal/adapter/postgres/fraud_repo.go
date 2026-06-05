package postgres

import (
	"context"
	"fmt"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FraudRepo is the pgx+sqlc adapter for usecase.FraudRepo (fraud_logs +
// user_trade_pairs).
type FraudRepo struct{ q *sqlc.Queries }

func NewFraudRepo(pool *pgxpool.Pool) *FraudRepo { return &FraudRepo{q: sqlc.New(pool)} }

var _ usecase.FraudRepo = (*FraudRepo)(nil)

func (r *FraudRepo) UpsertTradePair(ctx context.Context, u1, u2 uint, pair string, total float64) (usecase.TradePairCounters, error) {
	row, err := r.q.UpsertUserTradePair(ctx, sqlc.UpsertUserTradePairParams{
		User1ID: int64(u1), User2ID: int64(u2), Pair: pair, Total: total,
	})
	if err != nil {
		return usecase.TradePairCounters{}, fmt.Errorf("postgres: upsert trade pair: %w", err)
	}
	return usecase.TradePairCounters{TradeCount: int(row.TradeCount), TotalVol: row.TotalVol, FirstTrade: row.FirstTrade}, nil
}

func (r *FraudRepo) CountActiveByTypeUsers(ctx context.Context, fraudType, userIDs string) (int64, error) {
	return r.q.CountFraudByTypeUsersActive(ctx, sqlc.CountFraudByTypeUsersActiveParams{FraudType: fraudType, UserIds: userIDs})
}

func (r *FraudRepo) CountByTypeUsers(ctx context.Context, fraudType, userIDs string) (int64, error) {
	return r.q.CountFraudByTypeUsers(ctx, sqlc.CountFraudByTypeUsersParams{FraudType: fraudType, UserIds: userIDs})
}

func (r *FraudRepo) CreateFraudLog(ctx context.Context, l *domain.FraudLog) error {
	row, err := r.q.CreateFraudLog(ctx, sqlc.CreateFraudLogParams{
		UserIds: l.UserIDs, FraudType: l.FraudType, Description: l.Description,
		Evidence: l.Evidence, Action: defaultStr(l.Action, "FLAGGED"), AdminNote: l.AdminNote,
	})
	if err != nil {
		return fmt.Errorf("postgres: create fraud log: %w", err)
	}
	l.ID = uint(row.ID)
	l.CreatedAt = row.CreatedAt
	return nil
}

func (r *FraudRepo) ListFraudLogs(ctx context.Context, search string, page, size int) ([]domain.FraudLog, int64, error) {
	rows, err := r.q.ListFraudLogs(ctx, sqlc.ListFraudLogsParams{Search: search, Lim: int32(size), Off: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list fraud logs: %w", err)
	}
	total, err := r.q.CountFraudLogs(ctx, search)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count fraud logs: %w", err)
	}
	out := make([]domain.FraudLog, len(rows))
	for i, m := range rows {
		out[i] = domain.FraudLog{
			ID: uint(m.ID), UserIDs: m.UserIds, FraudType: m.FraudType, Description: m.Description,
			Evidence: m.Evidence, Action: m.Action, AdminNote: m.AdminNote, CreatedAt: m.CreatedAt,
		}
	}
	return out, total, nil
}

func (r *FraudRepo) UpdateFraudAction(ctx context.Context, logID uint, action, note string) error {
	return wrap("update fraud action", r.q.UpdateFraudAction(ctx, sqlc.UpdateFraudActionParams{ID: int64(logID), Action: action, AdminNote: note}))
}
