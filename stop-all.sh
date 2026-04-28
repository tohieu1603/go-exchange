#!/bin/bash
# Stop all CryptoX microservices
cd "$(dirname "$0")"

PID_FILE=".logs/.pids"
if [ -f "$PID_FILE" ]; then
  PIDS=$(cat "$PID_FILE")
  echo "Stopping PIDs: $PIDS"
  kill $PIDS 2>/dev/null
  rm -f "$PID_FILE"
  echo "All services stopped."
else
  echo "No PID file found. Killing by port..."
  for port in 8080 8081 8082 8083 8084 8085 8086; do
    lsof -ti :$port | xargs kill 2>/dev/null && echo "  Killed :$port"
  done
  echo "Done."
fi
