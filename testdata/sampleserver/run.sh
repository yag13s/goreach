#!/usr/bin/env bash
set -euo pipefail

# Sample server の E2E デモスクリプト
# プロジェクトルートから実行: bash testdata/sampleserver/run.sh

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
COVERDIR="/tmp/goreach-demo"
BIN="$ROOT/bin/sampleserver"
PORT=8080

echo "=== goreach demo ==="
echo ""

# Clean up
rm -rf "$COVERDIR"
mkdir -p "$COVERDIR"

# Build with coverage
echo "[1/5] Building sampleserver with -cover ..."
mkdir -p "$ROOT/bin"
go build -cover -covermode=atomic -o "$BIN" "$ROOT/testdata/sampleserver"

# Start server
echo "[2/5] Starting server (GOCOVERDIR=$COVERDIR) ..."
GOCOVERDIR="$COVERDIR" "$BIN" &
PID=$!
sleep 1

# Send requests (some paths will be hit, others won't)
echo "[3/5] Sending requests ..."
curl -s "http://localhost:$PORT/hello?name=Alice"
echo ""
curl -s "http://localhost:$PORT/hello?name=Bob"
echo ""
curl -s "http://localhost:$PORT/calc?op=add&a=10&b=20"
echo ""
curl -s "http://localhost:$PORT/calc?op=mul&a=3&b=7"
echo ""
curl -s "http://localhost:$PORT/health"
echo ""

# Stop server (SIGTERM triggers coverage write)
echo "[4/5] Stopping server (pid=$PID) ..."
kill -TERM "$PID"
wait "$PID" 2>/dev/null || true
sleep 1

# Analyze
echo "[5/5] Analyzing coverage ..."
echo ""
echo "--- Summary ---"
go run "$ROOT/cmd/goreach" summary -coverdir "$COVERDIR"
echo ""
echo "--- Unreached functions (threshold=50) ---"
go run "$ROOT/cmd/goreach" analyze -coverdir "$COVERDIR" -threshold 50 -pretty

echo ""
echo "Full report: go run ./cmd/goreach analyze -coverdir $COVERDIR -pretty"
