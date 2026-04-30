#!/usr/bin/env bash
# exchange-deploy.sh — runs ON the deploy host, streamed via Jenkins SSH stdin.
#
# Inputs (env vars passed via SSH command line):
#   BRANCH    — git branch to deploy (default: main)
#   GIT_SHA   — short sha for log/notification (informational)
#   ROLLBACK  — when set to 1, skip git-pull/build and restore the
#               previous binaries from <svc>/bin/<svc>.previous, restart,
#               and exit. Used by ops for fast manual recovery.
#
# Auto-rollback:
#   Each successful build snapshots the prior binary as <svc>.previous.
#   If the post-restart health gate fails, the script swaps each .previous
#   back into place, restarts again, and exits non-zero. Jenkins still
#   marks the run failed; production stays on the last-known-good build.
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
ROLLBACK="${ROLLBACK:-0}"

REPO_DIR="${REPO_DIR:-/srv/micro-exchange}"
HEALTH_URL="${HEALTH_URL:-http://localhost:8080/api/health}"

# Service↔systemd-unit mapping. Keep alphabetical so per-service iteration
# order is stable across grep/awk pipelines and log diffs.
SERVICES=(
  auth-service
  es-indexer
  futures-service
  gateway
  market-service
  notification-service
  trading-service
  wallet-service
)
unit_for() { echo "exchange-$1.service"; }

log() { printf '\n── %s ──\n' "$*"; }

restart_all() {
  for svc in "${SERVICES[@]}"; do
    sudo systemctl restart "$(unit_for "$svc")" || {
      echo "FAIL: $(unit_for "$svc") failed to restart"
      sudo systemctl status --no-pager "$(unit_for "$svc")" | tail -20
      return 1
    }
  done
}

health_wait() {
  # Poll /api/health up to 30s. DB warm-up + redis cache hydrate take a
  # few seconds; fail fast if not green by then.
  for i in $(seq 1 15); do
    if curl -fsS --max-time 2 "$HEALTH_URL" > /dev/null 2>&1; then
      log "health OK at sha=$GIT_SHA (try $i)"
      return 0
    fi
    sleep 2
  done
  return 1
}

dump_journal() {
  for svc in "${SERVICES[@]}"; do
    echo "── $(unit_for "$svc") (last 10 lines) ──"
    sudo journalctl -u "$(unit_for "$svc")" --no-pager -n 10
  done
}

rollback_binaries() {
  log "ROLLBACK: swapping in <svc>/bin/<svc>.previous"
  cd "$REPO_DIR"
  local missing=0
  for svc in "${SERVICES[@]}"; do
    local cur="$svc/bin/$svc"
    local prev="$svc/bin/$svc.previous"
    if [ ! -f "$prev" ]; then
      echo "  ! no previous for $svc — leaving current in place"
      missing=1
      continue
    fi
    mv "$prev" "$cur"
    echo "  $svc: restored"
  done
  if [ "$missing" = "1" ]; then
    echo "WARN: at least one service had no previous binary; partial rollback"
  fi
}

# ── Manual rollback path ───────────────────────────────────────────────────
# Triggered by ROLLBACK=1 on the SSH command line. Skips git/build entirely.
if [ "$ROLLBACK" = "1" ]; then
  log "manual rollback requested"
  rollback_binaries
  restart_all || { dump_journal; exit 1; }
  if health_wait; then
    log "rollback OK"
    exit 0
  fi
  dump_journal
  exit 1
fi

# ── Normal deploy ──────────────────────────────────────────────────────────
log "deploy start: branch=$BRANCH sha=$GIT_SHA"

# 1. Sync code. Hard-reset to origin so a dirty checkout (rare, but
#    happens after a botched manual edit on the host) cannot poison
#    the build with stale work-tree changes.
log "sync $REPO_DIR to origin/$BRANCH"
cd "$REPO_DIR"
git fetch --quiet origin "$BRANCH"
git reset --hard "origin/$BRANCH"

# 2. Build every service. Per-service binary lands in <svc>/bin/<svc>.
#    Before overwriting, snapshot the existing binary as <svc>.previous so
#    we have a known-good revert target if the new build fails health.
log "build binaries (parallel, snapshot-on-success)"
build_one() {
  local svc="$1"
  cd "$svc"
  mkdir -p bin
  if [ -f "bin/$svc" ]; then
    cp -f "bin/$svc" "bin/$svc.previous"
  fi
  go build -o "bin/$svc" ./cmd/...
}
export -f build_one
printf '%s\n' "${SERVICES[@]}" \
  | xargs -n1 -P4 -I{} bash -c 'build_one "$@"' _ {}

# 3. Restart units. systemctl restart returns immediately; rely on the
#    health curl below to confirm everything came back up.
log "restart systemd units"
restart_all || { dump_journal; exit 1; }

# 4. Health gate. On failure, attempt one rollback before giving up.
log "health check $HEALTH_URL"
if health_wait; then
  log "deploy OK at sha=$GIT_SHA"
  exit 0
fi

log "health did not turn green within 30s — attempting auto-rollback"
dump_journal
rollback_binaries
if restart_all && health_wait; then
  echo "FAIL: rolled back to previous build successfully (production preserved)"
  exit 1
fi
echo "FAIL: rollback also failed health check — manual intervention required"
dump_journal
exit 1
