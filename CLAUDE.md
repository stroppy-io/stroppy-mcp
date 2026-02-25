# stroppy-mcp

MCP (Model Context Protocol) server for [Stroppy](https://github.com/stroppy-io/stroppy) — exposes database stress testing capabilities to AI assistants via tool calls.

## Build

```bash
go build -o stroppy-mcp .
```

## Usage

Add to Claude Code MCP config (`~/.claude/settings.json` or project `.mcp.json`):

```json
{
  "mcpServers": {
    "stroppy": {
      "command": "/path/to/stroppy-mcp"
    }
  }
}
```

Requires `stroppy` binary accessible via: `STROPPY_BIN` env var, `./build/stroppy`, or `$PATH`.

## Tools

| Tool | Description |
|------|-------------|
| `stroppy_gen` | Scaffold a workspace from a preset (simple, tpcb, tpcc, tpcds, execute_sql) |
| `stroppy_run` | Execute a stress test, returns k6 metrics summary |
| `stroppy_validate` | Dry-run syntax check on a script |
| `read_k6_summary` | Parse k6 JSON summary into formatted metrics |
| `inspect_db` | Connect to PostgreSQL, show version/config/size |
| `list_presets` | List available workload presets |
| `read_file` | Read a project file (.ts/.sql/.yaml/.json, 100KB cap) |

## Resource

| URI | Description |
|-----|-------------|
| `stroppy://docs` | Full Stroppy documentation (embedded from llms-full.txt) |

## Architecture

- `main.go` — Server setup, tool/resource registration, stdio transport
- `exec.go` — Stroppy CLI binary discovery and command execution
- `tools_gen.go` — `stroppy_gen`, `list_presets` handlers
- `tools_run.go` — `stroppy_run`, `stroppy_validate` handlers
- `tools_inspect.go` — `inspect_db`, `read_k6_summary`, `read_file` handlers
- `tools_docs.go` — `stroppy://docs` resource handler (embeds llms-full.txt via go:embed)

## Dependencies

- `github.com/mark3labs/mcp-go` — Go MCP SDK (stdio transport)
- `github.com/jackc/pgx/v5` — PostgreSQL driver for `inspect_db`
