#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MCP_DIR="$(dirname "$SCRIPT_DIR")"

echo "Building stroppy-mcp binary (linux/amd64)..."
cd "$MCP_DIR"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$SCRIPT_DIR/stroppy-mcp" .

echo "Building Docker image..."
cd "$SCRIPT_DIR"
docker build -t stroppy-cleanroom .

echo ""
echo "Done. Run with:"
echo ""
echo "  docker run -it stroppy-cleanroom"
echo ""
echo "Or to persist reports on your host:"
echo ""
echo "  docker run -it -v \$(pwd)/reports:/workspace/reports stroppy-cleanroom"
