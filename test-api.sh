#!/bin/bash
# Quick end-to-end smoke test of Micro-Exchange API.
# Run AFTER infra + services are up: docker compose up -d && ./start-all.sh
#
# Usage: ./test-api.sh
# Cleanup: rm -f /tmp/cookies.txt
set -u

BASE="http://localhost:8080"
COOKIES="/tmp/cookies.txt"
EMAIL="${EMAIL:-test-$(date +%s)@example.com}"
PASS="Tg9k!7p2\$xQv"
HDR_JSON="-H Content-Type:application/json"

green()  { printf "\033[32m%s\033[0m\n" "$*"; }
red()    { printf "\033[31m%s\033[0m\n" "$*"; }
header() { printf "\n\033[1;36m═══ %s ═══\033[0m\n" "$*"; }

rm -f "$COOKIES"

header "1. Register"
curl -sS -X POST $BASE/api/auth/register $HDR_JSON \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"fullName\":\"Test User\"}" \
  -c "$COOKIES" | python3 -m json.tool 2>&1 | head -15

header "2. Login"
curl -sS -X POST $BASE/api/auth/login $HDR_JSON \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" \
  -c "$COOKIES" | python3 -m json.tool 2>&1 | head -10

USER_ID=$(curl -sS $BASE/api/auth/profile -b "$COOKIES" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null)

if [[ -z "$USER_ID" ]]; then
  red "Login failed — likely step-up required (new device)."
  red "Skipping balance/order tests."
  echo "→ To bypass step-up, complete /api/auth/step-up flow with the OTP code from .logs/auth.log"
  exit 1
fi
green "Logged in as user_id=$USER_ID"

header "3. Inject balance + KYC verify (DB direct)"
docker exec -e PGPASSWORD=postgres micro-exchange-pg-auth-1 psql -U postgres -d auth_db \
  -c "UPDATE users SET kyc_status='VERIFIED', kyc_step=4 WHERE id=$USER_ID" >/dev/null
docker exec -e PGPASSWORD=postgres micro-exchange-pg-wallet-1 psql -U postgres -d wallet_db \
  -c "UPDATE wallets SET balance=1000000 WHERE user_id=$USER_ID AND currency='USDT'" >/dev/null
docker exec micro-exchange-redis-1 redis-cli SET "kyc:$USER_ID" "4" >/dev/null
docker exec micro-exchange-redis-1 redis-cli SET "bal:$USER_ID:USDT" "1000000" >/dev/null
docker exec micro-exchange-redis-1 redis-cli SET "locked:$USER_ID:USDT" "0" >/dev/null
green "Balance: 1,000,000 USDT  KYC: VERIFIED"

header "4. List balances"
curl -sS $BASE/api/wallet/balances -b "$COOKIES" | python3 -c "
import sys,json
d = json.load(sys.stdin)['data']
for w in d:
    if float(w['balance']) > 0 or float(w['lockedBalance']) > 0:
        print(f\"  {w['currency']:6} balance={w['balance']} locked={w['lockedBalance']}\")"

header "5. Get tickers"
curl -sS $BASE/api/market/tickers | python3 -c "
import sys,json
d = json.load(sys.stdin).get('data', [])
for t in d[:3]:
    print(f\"  {t['pair']:12} \${t['price']:>10.2f}  Δ24h={t['change24h']:+.2f}%\")"

header "6. Place LIMIT BUY 0.0005 BTC @ \$50K (won't fill — locks balance)"
ORDER=$(curl -sS -X POST $BASE/api/trading/orders $HDR_JSON -b "$COOKIES" \
  -d '{"pair":"BTC_USDT","side":"BUY","type":"LIMIT","price":50000,"amount":0.0005}')
echo "$ORDER" | python3 -m json.tool | head -15
ORDER_ID=$(echo "$ORDER" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null)

header "7. Redis locked state"
docker exec micro-exchange-redis-1 redis-cli GET "locked:$USER_ID:USDT"

if [[ -n "$ORDER_ID" ]]; then
  header "8. Cancel order #$ORDER_ID"
  curl -sS -X DELETE $BASE/api/trading/orders/$ORDER_ID -b "$COOKIES" | python3 -m json.tool | head -5

  header "9. Redis unlocked state"
  docker exec micro-exchange-redis-1 redis-cli GET "locked:$USER_ID:USDT"
fi

header "10. MARKET BUY 0.0001 BTC at current price"
curl -sS -X POST $BASE/api/trading/orders $HDR_JSON -b "$COOKIES" \
  -d '{"pair":"BTC_USDT","side":"BUY","type":"MARKET","amount":0.0001}' \
  | python3 -m json.tool | head -15

header "11. Open Futures LONG 0.001 BTC 10x with TP/SL"
curl -sS -X POST $BASE/api/futures/order $HDR_JSON -b "$COOKIES" \
  -d '{"pair":"BTC_USDT","side":"LONG","leverage":10,"size":0.001,"takeProfit":85000,"stopLoss":70000}' \
  | python3 -m json.tool | head -15

header "12. Open positions"
curl -sS $BASE/api/futures/positions/open -b "$COOKIES" | python3 -m json.tool | head -20

header "13. Audit log"
curl -sS $BASE/api/auth/audit -b "$COOKIES" | python3 -c "
import sys,json
rows = json.load(sys.stdin)['data']['content']
for r in rows[:5]:
    print(f\"  [{r['action']:25s}] {r['outcome']:8s} @ {r['createdAt'][:19]}\")"

header "14. Logout"
curl -sS -X POST $BASE/api/auth/logout -b "$COOKIES" | python3 -m json.tool | head -3

green "Done. Detailed logs: .logs/<service>.log"
