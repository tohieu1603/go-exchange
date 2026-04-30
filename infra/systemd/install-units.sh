#!/usr/bin/env bash
# install-units.sh — renders exchange-template.service for all 8 services
# and installs them to /etc/systemd/system/. Run as root on the deploy host.
#
# Defaults match the Jenkinsfile deploy stage:
#   REPO_DIR=/home/oceanroot/exchange
#   ENV_DIR=/etc/exchange
#   RUN_USER=oceanroot
#
# Override by exporting before running, e.g.
#   RUN_USER=deploy ENV_DIR=/srv/exchange/env sudo -E ./install-units.sh

set -euo pipefail

REPO_DIR="${REPO_DIR:-/home/oceanroot/exchange}"
ENV_DIR="${ENV_DIR:-/etc/exchange}"
RUN_USER="${RUN_USER:-oceanroot}"

TEMPLATE="$(dirname "$0")/exchange-template.service"
[ -f "$TEMPLATE" ] || { echo "template not found: $TEMPLATE"; exit 1; }

# Each entry: <service-dir>:<unit-suffix>
# Service dir matches the repo subfolder; unit suffix matches the loop in
# Jenkinsfile (`for svc in auth wallet …`).
ENTRIES=(
  "auth-service:auth"
  "wallet-service:wallet"
  "market-service:market"
  "trading-service:trading"
  "futures-service:futures"
  "notification-service:notification"
  "gateway:gateway"
  "es-indexer:es-indexer"
)

mkdir -p "$ENV_DIR"

for entry in "${ENTRIES[@]}"; do
  svc="${entry%%:*}"
  unit_suffix="${entry##*:}"
  out="/etc/systemd/system/exchange-${unit_suffix}.service"
  env_file="$ENV_DIR/${svc}.env"
  echo "── render $out (svc=$svc)"
  sed \
    -e "s|{{SERVICE}}|${svc}|g" \
    -e "s|{{UNIT}}|${unit_suffix}|g" \
    -e "s|{{REPO_DIR}}|${REPO_DIR}|g" \
    -e "s|{{ENV_FILE}}|${env_file}|g" \
    -e "s|{{USER}}|${RUN_USER}|g" \
    "$TEMPLATE" > "$out"
  [ -f "$env_file" ] || echo "  ! missing $env_file — create it before starting the unit"
done

systemctl daemon-reload
echo
echo "Installed 8 exchange-*.service units."
echo "Next steps:"
echo "  1. Populate $ENV_DIR/<service>.env with the real env vars"
echo "     (see <svc>/.env.example in the repo)"
echo "  2. systemctl enable --now exchange-{auth,wallet,market,trading,futures,notification,gateway,es-indexer}"
echo "  3. Verify: systemctl status exchange-gateway"
