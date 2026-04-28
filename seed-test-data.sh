#!/bin/bash
# Seed test users into the running stack.
#
# Each user:
#   - Registered via /api/auth/register (so password is properly bcrypted)
#   - KYC status forced to VERIFIED (kyc_step=4) directly in postgres
#   - Wallet credited 10,000 USDT (postgres + redis cache mirrored)
#   - Redis kyc:<id> + bal:<id>:USDT + locked:<id>:USDT seeded so the trading
#     hot path Lua scripts see the balance immediately
#
# Prerequisites: docker compose up -d  &&  ./start-all.sh
# Usage: ./seed-test-data.sh [count]   (default 5)
set -u

COUNT=${1:-5}
BASE="http://localhost:8080"
PASS='Test@123456'                                  # passes the registration validator
USDT_AMOUNT=10000

green()  { printf "\033[32m%s\033[0m\n" "$*"; }
red()    { printf "\033[31m%s\033[0m\n" "$*"; }
header() { printf "\n\033[1;36m═══ %s ═══\033[0m\n" "$*"; }

# Wait for gateway to respond (skip if already up)
if ! curl -sf "$BASE/api/health" >/dev/null 2>&1; then
  red "Gateway not reachable at $BASE — start the stack first."
  exit 1
fi

header "Seeding 1 admin + $COUNT regular users (password='$PASS')"

declare -a CREDS

# ── Admin account ─────────────────────────────────────────────────────────
# Same flow as regular users + a final UPDATE that promotes role to ADMIN.
# admin@example.com is the conventional dev admin login; do not reuse in prod.
ADMIN_EMAIL="admin@example.com"
ADMIN_NAME="Platform Admin"
curl -sS -X POST "$BASE/api/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$PASS\",\"fullName\":\"$ADMIN_NAME\"}" >/dev/null 2>&1

ADMIN_ID=$(docker exec -e PGPASSWORD=postgres micro-exchange-pg-auth-1 \
  psql -U postgres -d auth_db -tAc "SELECT id FROM users WHERE email='$ADMIN_EMAIL'" 2>/dev/null)

if [[ -n "$ADMIN_ID" ]]; then
  docker exec -e PGPASSWORD=postgres micro-exchange-pg-auth-1 psql -U postgres -d auth_db -c \
    "UPDATE users SET role='ADMIN', kyc_status='VERIFIED', kyc_step=4 WHERE id=$ADMIN_ID" >/dev/null
  docker exec -e PGPASSWORD=postgres micro-exchange-pg-wallet-1 psql -U postgres -d wallet_db -c \
    "INSERT INTO wallets (user_id, currency, balance, locked_balance, created_at, updated_at)
     VALUES ($ADMIN_ID, 'USDT', $USDT_AMOUNT, 0, NOW(), NOW())
     ON CONFLICT (user_id, currency) DO UPDATE SET balance=$USDT_AMOUNT, locked_balance=0, updated_at=NOW()" \
    >/dev/null 2>&1
  docker exec micro-exchange-redis-1 redis-cli SET "kyc:$ADMIN_ID" "4" >/dev/null
  docker exec micro-exchange-redis-1 redis-cli SET "bal:$ADMIN_ID:USDT" "$USDT_AMOUNT" >/dev/null
  docker exec micro-exchange-redis-1 redis-cli SET "locked:$ADMIN_ID:USDT" "0" >/dev/null
  # Wipe audit history so the step-up gate treats next login as "first ever"
  # (no prior login.success → step-up skipped). Without this, a register-time
  # audit row + different login User-Agent ⇒ step-up modal pops up.
  docker exec -e PGPASSWORD=postgres micro-exchange-pg-auth-1 psql -U postgres -d auth_db -c \
    "DELETE FROM audit_logs WHERE user_id=$ADMIN_ID" >/dev/null
  CREDS+=("$ADMIN_ID|$ADMIN_EMAIL|$PASS|$USDT_AMOUNT USDT|VERIFIED|ADMIN")
  green "  ✓ user_id=$ADMIN_ID email=$ADMIN_EMAIL role=ADMIN"
else
  red "  Admin register failed — check stack logs."
fi

# ── Regular users ─────────────────────────────────────────────────────────
for i in $(seq 1 "$COUNT"); do
  EMAIL="seed${i}@example.com"
  NAME="Seed User $i"

  # 1) Register (idempotent: if email exists, server replies 400 — we still try DB lookup)
  REG=$(curl -sS -X POST "$BASE/api/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"fullName\":\"$NAME\"}" 2>/dev/null)

  # 2) Resolve user id from DB regardless (works for both fresh + existing users)
  USER_ID=$(docker exec -e PGPASSWORD=postgres micro-exchange-pg-auth-1 \
    psql -U postgres -d auth_db -tAc "SELECT id FROM users WHERE email='$EMAIL'" 2>/dev/null)

  if [[ -z "$USER_ID" ]]; then
    red "  [$EMAIL] register failed: $REG"
    continue
  fi

  # 3) Force KYC = VERIFIED (kyc_step 4 = post-document-approval)
  docker exec -e PGPASSWORD=postgres micro-exchange-pg-auth-1 psql -U postgres -d auth_db -c \
    "UPDATE users SET kyc_status='VERIFIED', kyc_step=4 WHERE id=$USER_ID" >/dev/null

  # 4) Ensure a USDT wallet row exists with the desired balance.
  #    Use ON CONFLICT to be safe across reseeds.
  docker exec -e PGPASSWORD=postgres micro-exchange-pg-wallet-1 psql -U postgres -d wallet_db -c \
    "INSERT INTO wallets (user_id, currency, balance, locked_balance, created_at, updated_at)
     VALUES ($USER_ID, 'USDT', $USDT_AMOUNT, 0, NOW(), NOW())
     ON CONFLICT (user_id, currency) DO UPDATE SET balance=$USDT_AMOUNT, locked_balance=0, updated_at=NOW()" \
    >/dev/null 2>&1

  # 5) Mirror in Redis — trading service reads this directly via Lua.
  docker exec micro-exchange-redis-1 redis-cli SET "kyc:$USER_ID" "4" >/dev/null
  docker exec micro-exchange-redis-1 redis-cli SET "bal:$USER_ID:USDT" "$USDT_AMOUNT" >/dev/null
  docker exec micro-exchange-redis-1 redis-cli SET "locked:$USER_ID:USDT" "0" >/dev/null

  # 6) Wipe audit history → step-up gate skips (no prior login.success row).
  docker exec -e PGPASSWORD=postgres micro-exchange-pg-auth-1 psql -U postgres -d auth_db -c \
    "DELETE FROM audit_logs WHERE user_id=$USER_ID" >/dev/null

  CREDS+=("$USER_ID|$EMAIL|$PASS|$USDT_AMOUNT USDT|VERIFIED|USER")
  green "  ✓ user_id=$USER_ID email=$EMAIL"
done

# Pretty table
header "Seeded credentials"
printf "%-4s %-22s %-16s %-13s %-9s %-6s\n" "ID" "EMAIL" "PASSWORD" "BALANCE" "KYC" "ROLE"
printf "%-4s %-22s %-16s %-13s %-9s %-6s\n" "----" "----------------------" "----------------" "-------------" "---------" "------"
for row in "${CREDS[@]}"; do
  IFS='|' read -r id em pw bal kyc role <<< "$row"
  printf "%-4s %-22s %-16s %-13s %-9s %-6s\n" "$id" "$em" "$pw" "$bal" "$kyc" "$role"
done

green "
Done. Login any account at http://localhost:3001/auth/login
- Admin user has full /admin/* access (sidebar visible after login)
- Regular users have KYC verified + 10K USDT — can place orders immediately"
