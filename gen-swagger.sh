#!/usr/bin/env bash
# Regenerate swagger docs for all services.
# Usage: ./gen-swagger.sh
set -e

SWAG="${SWAG:-$(go env GOPATH)/bin/swag}"

if ! command -v "$SWAG" &>/dev/null; then
  echo "swag not found — installing..."
  go install github.com/swaggo/swag/cmd/swag@latest
fi

ROOT="$(cd "$(dirname "$0")" && pwd)"

services=(
  auth-service
  wallet-service
  market-service
  trading-service
  futures-service
  notification-service
)

for svc in "${services[@]}"; do
  echo "==> $svc"
  (cd "$ROOT/$svc" && "$SWAG" init -g cmd/main.go -o cmd/docs)
done

echo "Done. Docs generated in each service's cmd/docs/"
