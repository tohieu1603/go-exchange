-- name: GetWallet :one
SELECT user_id, currency, balance, locked_balance
FROM wallets
WHERE user_id = $1 AND currency = $2;

-- name: ListWalletsByUser :many
SELECT user_id, currency, balance, locked_balance
FROM wallets
WHERE user_id = $1
ORDER BY currency;

-- name: ListAllWallets :many
SELECT user_id, currency, balance, locked_balance
FROM wallets;

-- name: CountWalletsByUser :one
SELECT count(*) FROM wallets WHERE user_id = $1;

-- name: InsertWallet :exec
-- Idempotent insert used by provisioning; an existing (user,currency) is left
-- untouched.
INSERT INTO wallets (user_id, currency, balance, locked_balance)
VALUES ($1, $2, $3, 0)
ON CONFLICT (user_id, currency) DO NOTHING;

-- name: UpsertWalletCredit :exec
-- Credit balance, creating the wallet row if it does not yet exist.
INSERT INTO wallets (user_id, currency, balance, locked_balance, updated_at)
VALUES (@user_id, @currency, @amount::numeric, 0, now())
ON CONFLICT (user_id, currency)
DO UPDATE SET balance = wallets.balance + EXCLUDED.balance, updated_at = now();

-- name: UpdateBalance :execrows
-- Add a signed delta. The guard makes a debit (delta < 0) affect zero rows when
-- it would overdraw; a credit always passes since balance and delta are both
-- non-negative in that case.
UPDATE wallets
SET balance = balance + @delta::numeric, updated_at = now()
WHERE user_id = @user_id AND currency = @currency AND balance + @delta::numeric >= 0;

-- name: LockBalance :execrows
-- Move amount from available into locked; affects zero rows (caller treats as
-- insufficient) when available < amount.
UPDATE wallets
SET locked_balance = locked_balance + @amount::numeric, updated_at = now()
WHERE user_id = @user_id AND currency = @currency
  AND (balance - locked_balance) >= @amount::numeric;

-- name: UnlockBalance :exec
-- Release amount from locked, floored at zero.
UPDATE wallets
SET locked_balance = CASE
        WHEN locked_balance - @amount::numeric < 0 THEN 0
        ELSE locked_balance - @amount::numeric
    END,
    updated_at = now()
WHERE user_id = @user_id AND currency = @currency;

-- name: IncreaseLocked :exec
-- Add amount to locked unconditionally (the availability check already happened
-- on the Redis hot path).
UPDATE wallets
SET locked_balance = locked_balance + @amount::numeric, updated_at = now()
WHERE user_id = @user_id AND currency = @currency;
