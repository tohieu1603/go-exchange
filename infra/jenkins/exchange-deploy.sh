#!/usr/bin/env bash
# exchange-deploy.sh — runs ON the deploy host, streamed via Jenkins SSH stdin.
#
# Inputs (env vars passed via SSH command line):
#   BRANCH   — git branch to deploy (default: main)
#   GIT_SHA  — short sha for log/notification (informational)
#
# Prerequisites on the host:
#   - $REPO_DIR contains a clone of the repo, owned by the deploy user.
#   - Go toolchain installed (matching go.work directive).
#   - 8 systemd units installed: exchange-{auth,wallet,market,trading,
#     futures,notification,es-indexer,gateway}.service
#     (templates in this repo at infra/systemd/).
#   - Health endpoint /api/health exposed by the gateway.

set -euo pipefail

BRANCH="${BRANCH:-main}"
GIT_SHA="${GIT_SHA:-unknown}"

REPO_DIR="${REPO_DIR:-/srv/micro-exchange}"
HEALTH_URL="${HEALTH_URL:-http://localhost:8080/api/health}"

SERVICES=(
  exchange-auth-service
  exchange-wallet-service
  exchange-market-service
  exchange-trading-service
  exchange-futures-service
  exchange-notification-service
  exchange-es-indexer
  exchange-gateway
)

log() { printf '\n── %s ──\n' "$*"; }

log "deploy start: branch=$BRANCH sha=$GIT_SHA"

# 1. Sync code. Hard-reset to origin so a dirty checkout (rare, but
#    happens after a botched manual edit on the host) cannot poison
#    the build with stale work-tree changes.
log "sync $REPO_DIR to origin/$BRANCH"
cd "$REPO_DIR"
git fetch --quiet origin "$BRANCH"
git reset --hard "origin/$BRANCH"

# 2. Build every service. Per-service binary lands in <svc>/bin/<svc>.
#    Build in parallel — independent go.mod files, ~3-4× wall-clock saved.
log "build binaries (parallel)"
build_one() {
  local svc="$1"
  ( cd "$svc" && go build -o "bin/$svc" ./cmd/... )
}
export -f build_one
printf '%s\n' \
  auth-service wallet-service market-service trading-service \
  futures-service notification-service es-indexer gateway \
  | xargs -n1 -P4 -I{} bash -c 'build_one "$@"' _ {}

# 3. Restart units. systemctl restart returns immediately; rely on the
#    health curl below to confirm everything came back up.
log "restart systemd units"
for unit in "${SERVICES[@]}"; do
  sudo systemctl restart "$unit" || {
    echo "FAIL: $unit failed to restart"
    sudo systemctl status --no-pager "$unit" | tail -20
    exit 1
  }
done

# 4. Health gate. Poll for up to 30s — services have a few seconds of
#    DB-connection + redis-cache warm-up before /api/health flips green.
log "health check $HEALTH_URL"
for i in $(seq 1 15); do
  if curl -fsS --max-time 2 "$HEALTH_URL" > /dev/null 2>&1; then
    log "deploy OK at sha=$GIT_SHA (try $i)"
    exit 0
  fi
  sleep 2
done

echo "FAIL: health check did not turn green within 30s"
for unit in "${SERVICES[@]}"; do
  echo "── $unit (last 10 lines) ──"
  sudo journalctl -u "$unit" --no-pager -n 10
done
exit 1
