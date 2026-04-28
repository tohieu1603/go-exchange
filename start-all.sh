#!/bin/bash
# Start all CryptoX microservices in background
# Each service connects to its own PostgreSQL database
# Usage: ./start-all.sh    Stop: Ctrl+C or ./stop-all.sh

cd "$(dirname "$0")"
ROOT=$(pwd)

export DB_USER=postgres
export DB_PASSWORD=postgres
export DB_HOST=localhost
export REDIS_URL=redis://localhost:6389
export JWT_SECRET=exchange-secret-change-in-production
export KAFKA_BROKERS=localhost:9192
export SEPAY_BANK_CODE=BIDV
export SEPAY_BANK_ACCOUNT=96247CISI1
export SEPAY_ACCOUNT_NAME="TO TRONG HIEU"
export WALLET_GRPC_ADDR=localhost:9082
export MARKET_GRPC_ADDR=localhost:9083
export AUTH_GRPC_ADDR=localhost:9081
export AUTH_URL=http://localhost:8081
export WALLET_URL=http://localhost:8082
export MARKET_URL=http://localhost:8083
export TRADING_URL=http://localhost:8084
export FUTURES_URL=http://localhost:8085
export NOTIFICATION_URL=http://localhost:8086

LOG_DIR="$ROOT/.logs"
mkdir -p "$LOG_DIR"
ALL_PIDS=""

start_svc() {
  local name="$1" dir="$2" port="$3" grpc="$4" dbport="$5" dbname="$6"
  echo "  $name :$port → pg:$dbport/$dbname"
  (
    cd "$ROOT/$dir"
    DB_PORT="$dbport" DB_NAME="$dbname" HTTP_PORT="$port" GRPC_PORT="${grpc:-0}" PORT="$port" \
      go run ./cmd/... > "$LOG_DIR/$name.log" 2>&1
  ) &
  ALL_PIDS="$ALL_PIDS $!"
}

echo "======================================="
echo "  CryptoX Microservices (6 databases)"
echo "======================================="

start_svc auth         auth-service          8081 9081 5551 auth_db
start_svc wallet       wallet-service        8082 9082 5552 wallet_db
start_svc market       market-service        8083 9083 5553 market_db
sleep 3
start_svc trading      trading-service       8084 ""   5554 trading_db
start_svc futures      futures-service       8085 ""   5555 futures_db
start_svc notification notification-service  8086 ""   5556 notification_db
start_svc gateway      gateway               8080 ""   0    none

# ES Indexer (headless worker, no HTTP port)
echo "  es-indexer (headless)"
cd "$ROOT/es-indexer"
ELASTIC_URL=http://localhost:9201 \
  go run ./cmd/... > "$LOG_DIR/es-indexer.log" 2>&1 &
ALL_PIDS="$ALL_PIDS $!"

echo ""
echo "All started! PIDs:$ALL_PIDS"
echo "Logs: $LOG_DIR/"
echo "$ALL_PIDS" > "$LOG_DIR/.pids"
echo ""
echo "  Gateway:  http://localhost:8080"
echo "  WS:       ws://localhost:8080/ws"
echo ""
echo "Ctrl+C to stop..."
trap "echo 'Stopping...'; kill $ALL_PIDS 2>/dev/null; wait; echo 'Done.'" INT TERM
wait
