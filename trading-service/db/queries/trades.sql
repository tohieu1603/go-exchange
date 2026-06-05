-- name: CreateTrade :one
INSERT INTO trades (pair, buy_order_id, sell_order_id, buyer_id, seller_id, price, amount, total, buyer_fee, seller_fee)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, created_at;
