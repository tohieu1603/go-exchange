#!/usr/bin/env bash
# install-units.sh — renders exchange-template.service for all 8 services
# and installs them to /etc/systemd/system/. Run as root on the deploy host.
#
# Defaults match Jenkinsfile + exchange-deploy.sh:
#   REPO_DIR=/srv/micro-exchange
#   ENV_DIR=/etc/exchange
#   USER=oceanroot
#
# Override by exporting before running, e.g.
#   USER=deploy ENV_DIR=/srv/exchange/env sudo -E ./install-units.sh

set -euo pipefail

REPO_DIR="${REPO_DIR:-/srv/micro-exchange}"
ENV_DIR="${ENV_DIR:-/etc/exchange}"
RUN_USER="${USER:-oceanroot}"

TEMPLATE="$(dirname "$0")/exchange-template.service"
[ -f "$TEMPLATE" ] || { echo "template not found: $TEMPLATE"; exit 1; }

SERVICES=(
  auth-service
  wallet-service
  market-service
  trading-service
  futures-service
  notification-service
  es-indexer
  gateway
)

mkdir -p "$ENV_DIR"

for svc in "${SERVICES[@]}"; do
  out="/etc/systemd/system/exchange-${svc}.service"
  env_file="$ENV_DIR/${svc}.env"
  echo "── render $out"
  sed \
    -e "s|{{SERVICE}}|${svc}|g" \
    -e "s|{{REPO_DIR}}|${REPO_DIR}|g" \
    -e "s|{{ENV_FILE}}|${env_file}|g" \
    -e "s|{{USER}}|${RUN_USER}|g" \
    "$TEMPLATE" > "$out"
  # Reminder: env file must be created by ops separately (not auto-generated
  # so secrets aren't accidentally committed to a config-management tool).
  [ -f "$env_file" ] || echo "  ! missing $env_file — create it before starting the unit"
done

systemctl daemon-reload
echo
echo "Installed 8 exchange-*.service units."
echo "Next steps:"
echo "  1. Populate $ENV_DIR/<service>.env with the real env vars"
echo "     (see <svc>/.env.example in the repo)"
echo "  2. systemctl enable --now exchange-<svc>.service for each"
echo "  3. Verify: systemctl status exchange-gateway.service"
