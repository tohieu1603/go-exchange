# Code Review: Full Security & Performance Audit — CryptoX Microservices

**Date:** 2026-03-22
**Reviewer:** Claude Code (claude-sonnet-4-6)
**Working Directory:** `/Users/admin/HieuTo/Base/micro-exchange`

---

## Scope

- **Files reviewed:** 85 Go files (~10,154 LOC) across 8 microservices + shared module; 3 TypeScript frontend files (~350 LOC)
- **Services:** auth, wallet, trading, futures, market, notification, gateway, es-indexer
- **Shared:** config, middleware, eventbus, ws/hub, redisutil (balance-cache, rate-limiter, lua-scripts), models, grpcutil
- **Frontend:** `frontendc/src/lib/api.ts`, `ws.ts`, `stores/auth-store.ts`
- **Review focus:** Full codebase — security, performance, code quality

---

## Overall Assessment

The codebase is **well-structured** with a clean CQRS pattern, atomic Redis Lua balance operations, per-pair mutex matching, and SELECT FOR UPDATE in futures close/liquidate. The main security posture is reasonable for an internal/demo system. However, there are **3 critical** and **8 high** severity issues that must be addressed before any production deployment, primarily around:

1. A panic-on-nil cast in the JWT middleware that can take down any service
2. Hardcoded default credentials seeded into production-reachable accounts
3. Webhook open-bypass in dev mode that is config-driven (no env = no auth)
4. Float64 financial arithmetic accumulated across all hot paths (trading, futures, PnL)
5. No DB connection pool configuration — GORM defaults will starve under load

---

## SECURITY

### CRITICAL

---

**[SEVERITY: CRITICAL]**
File: `shared/middleware/auth.go:44-46`
Issue: `GetUserID` performs an unchecked type assertion `id.(uint)`. If a handler is called outside the `JWTAuth` middleware context (e.g., a route misconfiguration, a future refactor), the context key `userId` is absent or `nil`, causing a **runtime panic that crashes the entire service** (gin.Recovery does not catch panics inside synchronous handler chains before they propagate).

```go
func GetUserID(c *gin.Context) uint {
    id, _ := c.Get("userId")
    return id.(uint)  // PANIC if userId not set
}
```

Fix:
```go
func GetUserID(c *gin.Context) uint {
    id, exists := c.Get("userId")
    if !exists {
        return 0
    }
    uid, ok := id.(uint)
    if !ok {
        return 0
    }
    return uid
}
```
All handlers calling `GetUserID` on routes that require auth already have `JWTAuth` applied, but the assertion must never panic — return 0 and let the handler reject.

---

**[SEVERITY: CRITICAL]**
File: `auth-service/internal/service/auth-service.go:253-276`
Issue: `SeedAdmin()` creates hardcoded demo accounts (`admin@exchange.com` / `admin123`, `user@exchange.com` / `user123`) with pre-funded wallets (100,000 USDT / 10,000 USDT). This runs on every cold-start if the users table is empty. In a production deployment, any operator that forgets to manually rotate these credentials leaves a permanently-open admin account.

```go
hash, _ := utils.HashPassword("admin123")
admin := &model.User{..., Role: "ADMIN", ...}
```

Fix: Seed should only run if an explicit env var `SEED_DEMO_DATA=true` is set, and should always log a **prominent warning** when used. Passwords must come from env vars, not be hardcoded.

---

**[SEVERITY: CRITICAL]**
File: `wallet-service/internal/handler/webhook-handler.go:82-85`
Issue: When neither `SEPAY_API_KEY` nor `SEPAY_SECRET_KEY` is configured (which is the default since `wallet-service/cmd/main.go:131-133` passes empty strings from `getEnv`), `validateWebhook` returns `true` unconditionally — **anyone can call the deposit-confirm webhook without authentication**. This allows arbitrary credit of USDT to any user account via a crafted `DEP-{userID}-{timestamp}` content field.

```go
// Dev mode: no keys configured
if h.apiKey == "" && h.secretKey == "" {
    return true  // OPEN TO WORLD
}
```

Fix: The fail-open dev mode must be removed. In production the service must fail-closed (reject) when no webhook secret is configured. If dev bypass is needed, gate it behind an explicit `WEBHOOK_AUTH_DISABLED=true` env var with a startup warning.

---

### HIGH

---

**[SEVERITY: HIGH]**
File: `shared/config/config.go:36`
Issue: All DB connections use `sslmode=disable` hardcoded. In a production PostgreSQL environment this exposes credentials and financial data to interception on the network.

Fix: Change default to `sslmode=require` and allow override via `DB_SSL_MODE` env var.

---

**[SEVERITY: HIGH]**
File: `auth-service/cmd/main.go:80`, `auth-service/cmd/main.go:80`
Issue: `POST /api/auth/2fa/login` has **no rate limiting**. An attacker with a valid temp token (15-min window) can brute-force the 6-digit TOTP code (1,000,000 combinations). The standard TOTP window is 30 seconds so ~2 valid codes exist at any time, but a fast brute-force still succeeds.

Fix: Apply `rl.GinMiddleware("2fa_login", time.Minute, 5)` to the 2FA login route matching the strictness of the regular login route.

---

**[SEVERITY: HIGH]**
File: `shared/redisutil/rate-limiter.go:32`
Issue: Rate limiter **fails open** on Redis errors — returns `true` (allow) when Redis is unavailable. For a financial exchange this means login brute-force, order spam, and deposit webhook calls go completely unthrottled during any Redis outage.

```go
if err != nil {
    return true // fail open on Redis errors
}
```

Fix: Fail closed for security-critical paths (login, 2FA, deposit). Use an in-process fallback (e.g., sync.Map token bucket) or return `false` with appropriate logging. At minimum, track Redis-unavailable state and emit a metric.

---

**[SEVERITY: HIGH]**
File: `shared/ws/hub.go:16-18`
Issue: WebSocket upgrader accepts **all origins**: `CheckOrigin: func(r *http.Request) bool { return true }`. This bypasses CORS protection for WebSocket connections (CORS headers don't apply to WS upgrades — only `CheckOrigin` does). Any page on the web can open a WebSocket to the exchange and subscribe to private channels if they obtain a token.

Fix:
```go
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        allowed := os.Getenv("CORS_ORIGINS")
        if allowed == "" {
            return origin == "http://localhost:3000" || origin == "http://localhost:3001"
        }
        for _, o := range strings.Split(allowed, ",") {
            if strings.TrimSpace(o) == origin {
                return true
            }
        }
        return false
    },
}
```

---

**[SEVERITY: HIGH]**
File: `shared/grpcutil/client.go:12`
Issue: All inter-service gRPC connections use `insecure.NewCredentials()` (plaintext). Internal service mesh traffic (wallet, trading, futures, gateway) can be intercepted or MITM'd on the container network. For a financial system, this is acceptable only if mTLS is handled at the infrastructure layer (e.g., Istio sidecar) — but there is no documented guarantee of that.

Fix: Document infrastructure-level mTLS requirement explicitly, or implement gRPC TLS using self-signed CA certs for service-to-service communication.

---

**[SEVERITY: HIGH]**
File: `shared/middleware/auth.go:31-41` + `gateway/cmd/main.go:79`
Issue: `AdminOnly()` middleware works correctly, but `gateway/cmd/main.go` marks `/api/admin/` as `auth: true` — the gateway validates the JWT but does **not** enforce the admin role check. The admin role check only happens at the auth-service HTTP layer. If the gateway JWT validation ever gets bypassed (service directly exposed), non-admin users can reach admin endpoints. More critically, the gateway passes `X-User-Role` in a header to backend services, but nothing prevents an external actor from injecting `X-User-Role: ADMIN` directly to a backend service if any backend port is reachable.

Fix: Each backend service must reject requests containing `X-User-Role` coming from non-gateway sources (e.g., enforce an internal shared secret header `X-Internal-Token` from gateway). The backends should derive role from the JWT they re-validate, not from forwarded headers.

---

**[SEVERITY: HIGH]**
File: `auth-service/cmd/main.go:80` + `auth-service/internal/service/auth-service.go:96`
Issue: When 2FA is enabled, `Login()` returns a `tempToken` generated by `GenerateAccessToken` — a standard 15-minute **access token** (not a restricted temp token type). A stolen temp token can be replayed as a full access token against any service that validates JWTs without checking the `requires2FA` context. There is no claim in the JWT to indicate it is a pre-2FA partial token.

Fix: Use a distinct JWT claim `"token_type": "2fa_pending"` in the temp token. `JWTAuth` middleware (and gRPC `ValidateToken`) must reject tokens with this type for all routes except `POST /auth/2fa/login`.

---

**[SEVERITY: HIGH]**
File: `wallet-service/internal/service/wallet-service.go:155`
Issue: In `ConfirmDeposit`, the Redis balance cache is credited (`s.balCache.Credit`) **outside** the database transaction. If the transaction later fails (e.g., `depositRepo.Save` fails), the Redis credit is not rolled back, resulting in a ghost balance increase that persists until Redis evicts the key or the balance is reconciled.

```go
return s.db.Transaction(func(tx *gorm.DB) error {
    ...
    s.balCache.Credit(ctx, deposit.UserID, "USDT", deposit.AmountUSDT)  // outside tx atomicity
    ...
    if err := s.depositRepo.Save(tx, deposit); err != nil {
        return err  // Redis credit NOT rolled back
    }
})
```

Fix: Move Redis cache update to after transaction commit success:
```go
err := s.db.Transaction(func(tx *gorm.DB) error { ... })
if err == nil {
    s.balCache.Credit(ctx, deposit.UserID, "USDT", deposit.AmountUSDT)
}
return err
```

---

### MEDIUM

---

**[SEVERITY: MEDIUM]**
File: `frontendc/src/lib/ws.ts:20`
Issue: JWT token is passed as a **URL query parameter** (`?token=<JWT>`). This exposes the token in server access logs, browser history, HTTP Referer headers, and CDN/proxy logs.

Fix: Send token via a one-time handshake message after connection upgrade:
```typescript
this.ws.onopen = () => {
    const token = localStorage.getItem('auth_token');
    if (token) this.ws?.send(JSON.stringify({ action: 'auth', token }));
    // then re-subscribe
};
```
Server-side, handle `action: 'auth'` in `readPump` to set `client.userID`.

---

**[SEVERITY: MEDIUM]**
File: `futures-service/internal/service/futures-service.go:81-93`
Issue: In `OpenPosition`, wallet deduction happens via gRPC **outside** any database transaction. If `positionRepo.Create` fails, the code attempts a best-effort refund (`_ = s.walletClient.Credit(...)`). If that refund call also fails (network partition), the user's balance is permanently deducted with no position record — **money is lost**.

Fix: Implement a saga pattern with explicit compensation, or use a two-phase approach: reserve the margin (lock, not deduct), create the position, then confirm deduction. Log and alert on compensation failures.

---

**[SEVERITY: MEDIUM]**
File: `shared/model/futures-position.go:25-29`, `trading-service/internal/service/matching-engine.go:121`
Issue: All financial calculations use `float64`. While the DB schema uses `decimal(30,10)`, Go computations use IEEE 754 double precision which has ~15-16 significant digits. For leveraged futures PnL calculations (`Size * (markPrice - entryPrice)`) with high leverage (125x), floating point drift can cause incorrect liquidation triggers or PnL miscalculations. Fee calculations (`total * 0.001`) are especially prone.

Fix: Use `github.com/shopspring/decimal` for all intermediate financial calculations. Convert to `float64` only for Redis storage (using the existing `"%.10f"` format string is adequate for Redis, but Go arithmetic must use decimal).

---

**[SEVERITY: MEDIUM]**
File: `market-service/internal/handler/market-handler.go:85`
Issue: `Trades` endpoint has no LIMIT validation for the `pair` parameter. Any string is passed directly to a GORM `Where("pair = ?", pair)` query. While GORM parameterizes this correctly (no SQL injection), there is no input validation on the pair format — an adversary can probe internal pair naming conventions and cause unnecessary DB work.

Also: `Candles` endpoint at line 96 validates interval but does not restrict to a whitelist — an unsupported interval string causes a DB query that returns no results with no error.

Fix: Validate `pair` against the `DefaultCoins` whitelist in the handler. Validate `interval` against `{"1m","5m","15m","1h","4h","1d","1w"}`.

---

**[SEVERITY: MEDIUM]**
File: `futures-service/internal/service/liquidation-engine.go:62-67`
Issue: `LiquidationEngine.Start()` spawns a goroutine with a ticker but no stop mechanism. The goroutine runs forever with no context cancellation or done channel. On graceful shutdown (`SIGTERM`), the liquidation engine continues running until the process exits, potentially executing liquidations against partially-shut-down state.

Fix:
```go
func (le *LiquidationEngine) Start(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                le.check()
            case <-ctx.Done():
                return
            }
        }
    }()
}
```

---

**[SEVERITY: MEDIUM]**
File: `wallet-service/internal/service/wallet-service.go:84-87`
Issue: `fetchRate()` creates a **new `http.Client` on every call** (every 5 minutes). While infrequent, this is a pattern that if replicated in higher-frequency code would cause fd leaks. The client does not reuse connections.

Fix: Create `http.Client` once as a package-level var or struct field.

---

**[SEVERITY: MEDIUM]**
File: `auth-service/internal/service/auth-service.go:253-254`
Issue: `utils.HashPassword` errors are silently swallowed during seeding:
```go
hash, _ := utils.HashPassword("admin123")
```
If bcrypt fails (extremely rare but possible), the admin account is seeded with an empty hash, making it unauthenticated-accessible.

Fix: `hash, err := utils.HashPassword("admin123"); if err != nil { log.Fatalf(...) }`.

---

### LOW

---

**[SEVERITY: LOW]**
File: `frontendc/src/lib/api.ts:23-28`
Issue: On 401, the client removes `auth_token` from localStorage and hard redirects — but does NOT clear the `refreshToken` (it's not stored in localStorage per the code, only `auth_token` is stored). If the refresh token was stored elsewhere or if `auth-store.ts` is extended, this creates a stale token scenario.

---

**[SEVERITY: LOW]**
File: `gateway/cmd/main.go:83`
Issue: Global rate limit of 300 req/min per IP is applied only via `r.NoRoute` which fires after route matching fails (no explicit match). All explicitly matched routes at the gateway level (e.g., `/health`, `/ws`) bypass rate limiting entirely.

Fix: Apply global rate limiting as `r.Use(...)` before route definitions, not inside `NoRoute`.

---

**[SEVERITY: LOW]**
File: `auth-service/cmd/main.go:80-81`
Issue: `POST /api/auth/refresh` has **no rate limiting**, allowing an attacker to rapidly probe whether a stolen refresh token hash is valid.

Fix: Apply `rl.GinMiddleware("refresh", time.Minute, 20)`.

---

## PERFORMANCE

### HIGH

---

**[SEVERITY: HIGH]**
File: All `*/cmd/main.go` services
Issue: **No database connection pool configuration** on any of the 6 PostgreSQL connections. GORM's default is `MaxOpenConns=0` (unlimited) and `MaxIdleConns=2`. Under concurrent load, this creates unbounded connections that exhaust PostgreSQL's `max_connections` (default 100) across 6 services simultaneously.

Fix: Configure pool on every DB instance:
```go
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(25)
sqlDB.SetMaxIdleConns(10)
sqlDB.SetConnMaxLifetime(5 * time.Minute)
sqlDB.SetConnMaxIdleTime(2 * time.Minute)
```

---

**[SEVERITY: HIGH]**
File: `futures-service/internal/service/liquidation-engine.go:71`
Issue: `check()` calls `positionRepo.FindAllOpen()` every 5 seconds — a **full table scan** of all open futures positions across all users. At scale (10,000+ open positions), this is an expensive unbounded query executed continuously.

`FindAllOpen` uses `SELECT ... WHERE status = 'OPEN'` — the `status` field has an index (`gorm:"default:OPEN;index"`), so the query will use an index scan, which is acceptable. However, the Redis price fetch inside the loop (`le.getPrice(pos.Pair)`) issues **one Redis GET per position per pair**. For N positions across P pairs, this is O(N) Redis calls every 5 seconds.

Fix: Batch price fetches by pair before iterating positions:
```go
prices := make(map[string]float64)
for i := range positions {
    pair := positions[i].Pair
    if _, seen := prices[pair]; !seen {
        prices[pair] = le.getPrice(pair)
    }
}
```

---

**[SEVERITY: HIGH]**
File: `trading-service/internal/repository/order-repo.go:79-81`
Issue: `FindOpenLimitOrders()` — used for cold-start recovery — has **no LIMIT clause** and fetches all open LIMIT orders across all users and all pairs. In production with millions of historical orders, this could OOM the service on startup.

```go
err := r.db.Where("status IN ? AND type = ?", []string{"OPEN", "PARTIAL"}, "LIMIT").Find(&orders).Error
```

Fix: This is a cold-start recovery function, so all open orders need loading — but add a defensive `Limit(50000)` with a warning if more exist, or implement paginated loading into the order book.

---

### MEDIUM

---

**[SEVERITY: MEDIUM]**
File: `futures-service/internal/service/liquidation-engine.go:125`
Issue: N+1 query pattern in `checkAccountMargin`. For each unique `userID` in open positions, a separate `walletRepo.FindByUserAndCurrency(userID, "USDT")` DB call is made. With U unique users, this is U sequential DB queries per liquidation check cycle (every 5 seconds).

Fix: Collect all user IDs first, then batch-load wallets:
```go
userIDs := make([]uint, 0, len(userPositions))
for uid := range userPositions { userIDs = append(userIDs, uid) }
wallets, _ := le.walletRepo.FindByUserIDsAndCurrency(userIDs, "USDT")
walletMap := map[uint]*model.Wallet{}
for i := range wallets { walletMap[wallets[i].UserID] = &wallets[i] }
```

---

**[SEVERITY: MEDIUM]**
File: `trading-service/internal/service/order-service.go:40`
Issue: `CancelOrder` calls `FindByID` (loads full order) then `Save` (full model update). For cancellation, only `status` needs updating — a targeted `UPDATE orders SET status='CANCELLED' WHERE id=? AND user_id=? AND status IN ('OPEN','PARTIAL')` is more efficient and also eliminates the TOCTOU race between load and save.

---

**[SEVERITY: MEDIUM]**
File: `shared/redisutil/balance-cache.go:30-33`
Issue: `LoadFromDB` uses `Set` with `0` TTL (no expiry) for balance cache keys. Redis balance keys never expire, accumulating indefinitely as the user base grows. With no TTL there is no memory bound.

Fix: Set a reasonable TTL (e.g., 24 hours) with a sliding refresh on each access via `EXPIRE` in the Lua scripts, or use `SET ... EX 86400` with refresh-on-hit. Alternatively, for a hot-path cache, accept permanent residence but document memory sizing requirements.

---

**[SEVERITY: MEDIUM]**
File: `market-service/internal/service/price-feed.go:132`
Issue: `pollForexRates` spawns one goroutine per FX pair for random-walk simulation: `go func(fx *fxPair)`. These goroutines contain `for { ... time.Sleep(30 * time.Second) ... }` infinite loops with no stop channel. They never terminate.

Fix: Use a single ticker goroutine that iterates all pairs, or pass a context:
```go
go func(fx *fxPair, ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for { select { case <-ticker.C: ...; case <-ctx.Done(): return } }
}(pair, ctx)
```

---

**[SEVERITY: MEDIUM]**
File: `trading-service/internal/engine/orderbook.go` (via `matching-engine.go:109`)
Issue: `book.Match(order)` is called while the `pairLock` is held, but `OrderBook.AddOrder`/`RemoveOrder` also acquire `ob.mu`. The ordering is: `pairLock → ob.mu`. If any other code path acquires `ob.mu` first and then tries to acquire a `pairLock`, there is a **deadlock potential**. The current code does not appear to do this, but it is one refactoring step away.

Fix: Document the lock ordering explicitly: `pairLocks[pair]` must always be acquired before `ob.mu`. Enforce via code comments and a locking convention doc.

---

**[SEVERITY: MEDIUM]**
File: `shared/eventbus/eventbus.go` (Redis Streams) + `kafka-adapter.go`
Issue: `Publish` is called per-event in the matching engine hot path: up to 8+ `bus.Publish` calls per trade (4 balance events + 2 order events + 1 trade event + WS broadcast). Each Redis Streams `XADD` is a round-trip. Under high trade volume this introduces latency in the critical matching path.

Fix: Buffer events and publish in batch after all trade processing using the existing `PublishBatch` Kafka method. For Redis Streams, use a pipeline or implement a micro-batch flush.

---

## CODE QUALITY

### HIGH

---

**[SEVERITY: HIGH]**
File: `shared/middleware/auth.go:33-40` — `AdminOnly()` depends on `JWTAuth` having run first. If `AdminOnly()` is ever applied without `JWTAuth` in a middleware chain, `role` will be nil and the middleware will block all requests (returns 403). This is a silent correctness dependency with no enforcement.

Fix: Inside `AdminOnly()`, verify that the `userId` context key exists to detect misconfigured middleware chains:
```go
if _, hasAuth := c.Get("userId"); !hasAuth {
    c.JSON(500, gin.H{"message": "auth middleware not applied"})
    c.Abort()
    return
}
```

---

**[SEVERITY: HIGH]**
File: `wallet-service/internal/service/wallet-service.go:200-212`
Issue: `UpdateBalance` — the Redis cache deduction happens **before** the DB update. On DB failure, it rolls back the Redis change. However, if the Redis rollback itself fails (`Credit` on rollback returns an error that is ignored), the Redis balance is permanently lower than the DB balance. This is a compensating transaction with no error handling on the compensation.

```go
if err := s.walletRepo.UpdateBalance(tx, ...); err != nil {
    if delta < 0 {
        s.balCache.Credit(ctx, ...) // error ignored
    }
    return err
}
```

Fix: Log rollback failures as CRITICAL alerts. Implement a reconciliation background job that periodically syncs Redis cache from DB for accounts where a discrepancy is detected.

---

### MEDIUM

---

**[SEVERITY: MEDIUM]**
File: `wallet-service/internal/service/wallet-service.go:322-328`
Issue: `CreateWithdrawal` calls `withdrawalRepo.FindLatestPendingByUser(userID)` after creating the withdrawal — a second DB round-trip — only to return the created record. The transaction already has the record in `withdrawal` local variable. This is unnecessary.

Fix: Return `withdrawal` directly from the transaction closure.

---

**[SEVERITY: MEDIUM]**
File: `trading-service/internal/handler/trading-handler.go:81`
Issue: The `_ = h.walletClient.CheckBalance(ctx, userID, quote, 0)` call (with amount=0) is used purely to warm the Redis cache. Passing 0 as the required amount makes the intent opaque. This is a side-effect-driven warm-up with no documentation.

Fix: Create a dedicated `WarmBalanceCache(ctx, userID, currency)` gRPC method or document this call with a clear comment: `// warms Redis balance cache via gRPC — must precede direct Lua lock`.

---

**[SEVERITY: MEDIUM]**
File: `auth-service/internal/service/admin-service.go:19-31` + `auth-service/internal/repository/user-repo.go:53-65`
Issue: Both `AdminService.GetUsers` and `userRepo.FindPaginated` implement nearly identical search+paginate logic. The `Count` query and `Find` query run on the same `q` builder, but `q.Count(&total)` mutates the builder's internal state — if the count fails, `Find` may still proceed on a partial builder. The `q.Count()` error is ignored in both places.

Fix: Check `q.Count(&total).Error` and return early on error. Deduplicate the logic into a shared repository method.

---

**[SEVERITY: MEDIUM]**
File: `market-service/internal/handler/market-handler.go:85`
Issue: `h.db.Where("pair = ?", pair).Order("created_at DESC").Limit(limit).Find(&trades)` — the error from this query is silently discarded. If the DB is unavailable, the handler returns an empty slice with HTTP 200, which the client interprets as "no trades" rather than a transient error.

Fix:
```go
if err := h.db.Where(...).Find(&trades).Error; err != nil {
    response.Error(c, 500, "failed to fetch trades")
    return
}
```

---

**[SEVERITY: MEDIUM]**
File: `wallet-service/internal/service/wallet-service.go:75-82`
Issue: `refreshRateLoop` goroutine is spawned in `NewWalletService` constructor with no shutdown path. The goroutine holds a reference to the service and runs indefinitely. There is no way to stop it for testing or graceful shutdown.

Fix: Accept a `context.Context` in the constructor or provide a `Stop()` method that cancels the rate refresh ticker.

---

**[SEVERITY: MEDIUM]**
File: `shared/eventbus/eventbus.go` (all consumer goroutines)
Issue: Consumer goroutines check `<-ctx.Done()` only at the top of the loop before the blocking `XReadGroup` call. If `XReadGroup` blocks for up to `3s` (the configured block time), the goroutine won't notice context cancellation for up to 3 seconds. Acceptable in most cases, but on shutdown this delays service termination.

---

### LOW

---

**[SEVERITY: LOW]**
File: `shared/config/config.go:18-19`
Issue: `_ = godotenv.Load()` silently ignores `.env` load errors. In a Kubernetes/Docker environment where `.env` is intentionally absent, this is fine. But if a malformed `.env` file is present, it will be silently ignored, potentially causing confusing missing-config bugs.

---

**[SEVERITY: LOW]**
File: `shared/ws/hub.go:186-190`
Issue: When a client's send buffer is full, the hub attempts to send to `h.unregister` channel non-blocking. If `unregister` is also full (very busy hub), the slow client is silently ignored rather than disconnected. This can cause message buildup until the client's goroutine eventually exits.

---

**[SEVERITY: LOW]**
File: `trading-service/internal/engine/orderbook.go` — insertion sort in `addToBids`/`addToAsks` is O(n) per insert where n = number of price levels. For a sparse order book this is fine, but for very active markets with many price levels, this becomes a bottleneck compared to a sorted tree structure.

---

**[SEVERITY: LOW]**
File: `frontendc/src/lib/api.ts:105`
Issue: String concatenation in URL construction: `` `/futures/positions${status ? '?status=' + status : ''}` `` — `status` is not URL-encoded. If status contains special characters (unlikely with enum values but possible with future extension), this could break the URL.

Fix: Use `URLSearchParams` consistently.

---

## Positive Observations

1. **Atomic Redis Lua scripts** (`lua-scripts.go`) for all balance operations — correctly prevents race conditions at the hot path. The deduct/credit/lock/unlock/transfer scripts are well-designed with proper key parameterization.

2. **Per-pair mutex** in `MatchingEngine.pairLocks` correctly serializes matching for each trading pair without blocking other pairs — good concurrency design.

3. **SELECT FOR UPDATE** in `futures-service/internal/repository/position-repo.go:55-59` — `FindByUserAndIDForUpdate` correctly uses `clause.Locking{Strength: "UPDATE"}` to prevent race conditions on position close.

4. **Webhook HMAC validation** in `webhook-handler.go:76-79` uses `hmac.Equal` (constant-time comparison) correctly — no timing attack vulnerability.

5. **Refresh token invalidation** via Redis with SHA-256 keying (`auth-service.go:242-244`) — correct revocation mechanism on logout.

6. **Graceful shutdown** implemented in all services using `signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)` with context timeouts.

7. **CORS middleware** (`cors.go`) correctly implements origin allowlist with exact-match (not wildcard) and `Vary: Origin` header.

8. **WebSocket private channel access control** (`hub.go:41-59`) correctly enforces `prefix@{userID}` format matching against authenticated `client.userID`.

9. **bcrypt cost factor 12** (`hash.go:6`) — above the recommended minimum of 10.

10. **Deposit idempotency** — `OrderCode` has a `uniqueIndex` in the DB model, preventing double-processing of the same deposit notification.

---

## Recommended Actions (Priority Order)

1. **[CRITICAL]** Fix `GetUserID` type assertion panic — add nil/type check, return 0 on failure.
2. **[CRITICAL]** Remove hardcoded `admin123`/`user123` seed credentials — gate behind env var, read passwords from env.
3. **[CRITICAL]** Remove webhook fail-open dev mode — fail-closed when no keys configured; require explicit env flag for bypass.
4. **[HIGH]** Fix Redis credit outside transaction in `ConfirmDeposit` — move credit to post-commit.
5. **[HIGH]** Add rate limiting to `2fa/login` and `refresh` endpoints.
6. **[HIGH]** Fix rate limiter fail-open — add in-process fallback for login/deposit paths.
7. **[HIGH]** Restrict WebSocket `CheckOrigin` to configured allowed origins.
8. **[HIGH]** Add DB connection pool configuration to all 6 services.
9. **[HIGH]** Add `token_type: 2fa_pending` claim to temp tokens; validate in `JWTAuth`.
10. **[HIGH]** Add N+1 batch wallet query in liquidation `checkAccountMargin`.
11. **[MEDIUM]** Switch financial calculations to `shopspring/decimal` — especially PnL, fee, and margin.
12. **[MEDIUM]** Add context-aware shutdown to `LiquidationEngine.Start()` and `refreshRateLoop`.
13. **[MEDIUM]** Change DB SSL default from `disable` to `require`.
14. **[MEDIUM]** Move WS token from URL query param to post-connect auth message.
15. **[MEDIUM]** Fix best-effort compensation in `OpenPosition` — implement saga with alerting on compensation failure.

---

## Metrics

- **Total Go files:** 85
- **Total Go LOC:** ~10,154
- **TypeScript files reviewed:** 3 (~350 LOC)
- **Critical issues:** 3
- **High severity:** 8
- **Medium severity:** 12
- **Low severity:** 6
- **DB connection pool configured:** 0 of 6 services
- **Rate-limited endpoints:** login (10/min), register (5/min) — missing: 2fa/login, refresh, deposit, order placement
- **Services with graceful shutdown:** 7/8 (market-service `cmd/main.go` does not implement graceful HTTP shutdown — it uses `log.Fatal` on listen error rather than signal-driven shutdown)

---

## Unresolved Questions

1. Are the gRPC ports (9081-9086) exposed outside the container network? If yes, the insecure gRPC transport becomes a critical issue requiring immediate TLS implementation.
2. Is there a separate reconciliation job to sync Redis balance cache with PostgreSQL? The current CQRS projector consumes `balance.changed` events but a Redis crash between event publish and DB write could leave the two out of sync with no detection mechanism.
3. Is `market-service` intentionally missing graceful shutdown? It uses `log.Fatal` on HTTP serve error which does not drain in-flight requests.
4. The `EnsureUSDTWallets()` function in auth-service seeds 10,000 USDT wallets for all existing users on every restart — is this intentional for a demo environment or should it be removed for production?
