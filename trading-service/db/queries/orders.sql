-- name: CreateOrder :one
INSERT INTO orders (user_id, pair, side, type, price, stop_price, amount, filled_amount, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, user_id, pair, side, type, price, stop_price, amount, filled_amount, status, created_at, updated_at;

-- name: GetOrderByID :one
SELECT id, user_id, pair, side, type, price, stop_price, amount, filled_amount, status, created_at, updated_at
FROM orders
WHERE id = $1;

-- name: GetOrderByUserAndID :one
SELECT id, user_id, pair, side, type, price, stop_price, amount, filled_amount, status, created_at, updated_at
FROM orders
WHERE id = $1 AND user_id = $2;

-- name: UpdateOrderStatus :exec
-- Apply a status/fill change. A MARKET order rests at price 0; seed its price
-- from the fill so history shows the executed price.
UPDATE orders
SET status = @status, filled_amount = @filled_amount::numeric,
    price = CASE WHEN price = 0 THEN @price::numeric ELSE price END,
    updated_at = now()
WHERE id = @id;

-- name: SaveOrder :exec
UPDATE orders
SET pair = $2, side = $3, type = $4, price = $5, stop_price = $6, amount = $7,
    filled_amount = $8, status = $9, updated_at = now()
WHERE id = $1;

-- name: FindOpenOrders :many
SELECT id, user_id, pair, side, type, price, stop_price, amount, filled_amount, status, created_at, updated_at
FROM orders
WHERE user_id = $1 AND status IN ('OPEN', 'PARTIAL')
ORDER BY created_at DESC;

-- name: FindOpenLimitOrders :many
SELECT id, user_id, pair, side, type, price, stop_price, amount, filled_amount, status, created_at, updated_at
FROM orders
WHERE status IN ('OPEN', 'PARTIAL') AND type = 'LIMIT';

-- name: ListOrdersByUser :many
-- An empty @status disables the status filter.
SELECT id, user_id, pair, side, type, price, stop_price, amount, filled_amount, status, created_at, updated_at
FROM orders
WHERE user_id = @user_id AND (@status::text = '' OR status = @status::text)
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountOrdersByUser :one
SELECT count(*)
FROM orders
WHERE user_id = @user_id AND (@status::text = '' OR status = @status::text);
