#!/bin/bash
# Imports Kibana saved objects (data view + saved searches + dashboard) for the
# Micro-Exchange security operations panel.
#
# Idempotent: Kibana's _import API supports overwrite=true. Safe to re-run.
#
# Usage: ./import-kibana-dashboards.sh
# Run AFTER `docker compose up -d` and once Kibana is healthy.

set -euo pipefail

KIBANA_URL="${KIBANA_URL:-http://localhost:5601}"
NDJSON="docs/kibana/security-dashboard.ndjson"

# Wait until Kibana reports green/yellow status. Without this, the API
# may return 503 because plugins are still initializing.
echo "Waiting for Kibana at $KIBANA_URL ..."
for i in $(seq 1 60); do
  status=$(curl -sS "$KIBANA_URL/api/status" 2>/dev/null | grep -oE '"level":"[^"]+"' | head -1 || true)
  if echo "$status" | grep -qE 'available|healthy|degraded'; then
    echo "  ready ($status)"
    break
  fi
  sleep 2
done

# Import via Kibana API. `kbn-xsrf: true` is mandatory for state-changing calls.
echo "Importing $NDJSON ..."
response=$(curl -sS -X POST "$KIBANA_URL/api/saved_objects/_import?overwrite=true" \
  -H "kbn-xsrf: true" \
  --form file=@$NDJSON)

# Parse success / failure from JSON
if echo "$response" | grep -q '"success":true'; then
  count=$(echo "$response" | grep -oE '"successCount":[0-9]+' | head -1 | grep -oE '[0-9]+')
  echo "Imported $count saved objects."
  echo
  echo "Open: $KIBANA_URL/app/dashboards#/view/audit-dashboard-security"
else
  echo "Import failed:"
  echo "$response"
  exit 1
fi
