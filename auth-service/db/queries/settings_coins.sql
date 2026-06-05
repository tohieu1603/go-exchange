-- name: GetPlatformSettings :one
SELECT id, deposit_fee_percent, withdraw_fee_percent, min_deposit, max_deposit, min_withdraw, max_withdraw, trading_fee_percent, kyc_required
FROM platform_settings WHERE id = $1;

-- name: UpsertPlatformSettings :exec
-- Single-row settings (id is always 1). Insert-or-replace all fields.
INSERT INTO platform_settings (id, deposit_fee_percent, withdraw_fee_percent, min_deposit, max_deposit, min_withdraw, max_withdraw, trading_fee_percent, kyc_required)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
    deposit_fee_percent = EXCLUDED.deposit_fee_percent,
    withdraw_fee_percent = EXCLUDED.withdraw_fee_percent,
    min_deposit = EXCLUDED.min_deposit,
    max_deposit = EXCLUDED.max_deposit,
    min_withdraw = EXCLUDED.min_withdraw,
    max_withdraw = EXCLUDED.max_withdraw,
    trading_fee_percent = EXCLUDED.trading_fee_percent,
    kyc_required = EXCLUDED.kyc_required;

-- name: CountCoins :one
SELECT count(*) FROM coins;

-- name: InsertCoin :exec
INSERT INTO coins (symbol, name, coin_gecko_id, bybit_symbol, icon_url, is_active, sort_order, asset_type)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (symbol) DO NOTHING;
