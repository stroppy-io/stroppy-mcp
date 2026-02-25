#!/bin/bash
set -e

# Start PostgreSQL
pg_ctlcluster 16 main start

# Set password on first run
su - postgres -c "psql -c \"ALTER USER postgres PASSWORD 'postgres';\"" 2>/dev/null || true

# Scaffold workspace if empty
if [ ! -f /workspace/tpcb.ts ]; then
    stroppy gen --workdir /workspace --preset=tpcb
    stroppy gen --workdir /workspace --preset=tpcc
fi

# Write .mcp.json in workspace so claude picks it up
cat > /workspace/.mcp.json <<'EOF'
{
  "mcpServers": {
    "stroppy": {
      "command": "stroppy-mcp",
      "env": {
        "STROPPY_BIN": "/usr/local/bin/stroppy"
      }
    }
  }
}
EOF

echo ""
echo "=== Stroppy Clean Room ==="
echo ""
echo "  PostgreSQL:   postgres://postgres:postgres@localhost:5432?sslmode=disable"
echo "  Stroppy:      $(stroppy version 2>&1)"
echo "  Claude Code:  $(claude --version 2>&1 | head -1)"
echo "  MCP server:   stroppy-mcp"
echo "  Workspace:    /workspace"
echo "  MCP config:   /workspace/.mcp.json"
echo ""
echo "  To start:     claude"
echo "  Try asking:   'run a TPC-C benchmark'"
echo ""

exec "$@"
