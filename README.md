# stroppy-mcp

MCP server that gives AI assistants direct access to [Stroppy](https://github.com/stroppy-io/stroppy), a k6-based database stress testing tool. Instead of constructing shell commands and parsing terminal output, the assistant calls structured tools — inspect a database, scaffold a test workspace, run a benchmark, read the results.

```
AI assistant  ──>  stroppy-mcp (MCP stdio)  ──>  stroppy CLI  ──>  PostgreSQL
```

## Why MCP instead of bash

An AI assistant can already run stroppy via shell commands. The MCP wrapper changes three things:

**No permission friction.** Every unique bash command requires user approval in Claude Code. A parameter sweep — 10 benchmark runs with different VU counts — means 10 approval prompts. MCP tools are approved once via config; all subsequent calls go through uninterrupted. This matters because the value of benchmarking is in iteration, and iteration dies when every step requires a click.

**Structured parameters, structured results.** The assistant doesn't need to remember k6 flag syntax, env var names, or output formats. `stroppy_run(script="tpcc.ts", env="VUS_SCALE=3", duration="30s")` is unambiguous. The returned metrics are machine-readable without grep.

**Natural integration with the rest of the assistant's capabilities.** MCP puts the assistant in the benchmarking loop. Once there, it brings everything else it can do — reading test scripts to discover parameters, editing them to fix bottlenecks (like a hardcoded connection pool size), reasoning about why throughput plateaued, suggesting PostgreSQL config changes. The tool calls and the code edits aren't separate workflows; they're one conversation.

## Tools

| Tool | What it does |
|------|-------------|
| `inspect_db` | Connect to PostgreSQL, return version, tuning parameters, database size |
| `stroppy_gen` | Scaffold a test workspace from a preset (tpcb, tpcc, tpcds, simple, execute_sql) |
| `stroppy_run` | Execute a stress test with configurable concurrency, duration, env vars, HTML report output |
| `stroppy_validate` | Dry-run transpile check — catch script errors before a real run |
| `list_presets` | List available workload presets with descriptions |
| `read_file` | Read project files (.ts, .sql, .yaml, .json, etc.) up to 100KB |
| `read_k6_summary` | Parse a k6 JSON summary into formatted latency percentiles, throughput, error rates |

## Resource

| URI | Description |
|-----|-------------|
| `stroppy://docs` | Full Stroppy documentation, embedded in the binary |

## Quick start

### Prerequisites

- **PostgreSQL 14+** — reachable via a `postgres://` connection URL
- **Stroppy CLI** — [download a release](https://github.com/stroppy-io/stroppy/releases) or build from source

### Build

```bash
go build -o stroppy-mcp .
```

### Configure

Add to Claude Code settings (`~/.claude/settings.json` or project `.mcp.json`):

```json
{
  "mcpServers": {
    "stroppy": {
      "command": "/absolute/path/to/stroppy-mcp",
      "env": {
        "STROPPY_BIN": "/usr/local/bin/stroppy"
      }
    }
  }
}
```

The server finds the stroppy binary via: `STROPPY_BIN` env var → `./build/stroppy` → `stroppy` on `$PATH`.

To skip per-tool approval prompts, add the wildcard permission:

```json
{
  "permissions": {
    "allow": ["mcp__stroppy__*"]
  }
}
```

### Use

```
You: "benchmark my postgres install"

Assistant calls inspect_db → sees stock config (shared_buffers=128MB on a 64GB machine)
Assistant calls stroppy_gen with tpcc preset → scaffolds workspace
Assistant calls stroppy_run → gets baseline: 9,700 TPS at 11 VUs
Assistant suggests ALTER SYSTEM SET shared_buffers = '16GB' ...
Assistant calls stroppy_run again → compares before/after
```

## Project structure

```
main.go            Server setup, tool/resource registration, stdio transport
exec.go            Stroppy binary discovery and command execution
tools_gen.go       stroppy_gen, list_presets
tools_run.go       stroppy_run, stroppy_validate
tools_inspect.go   inspect_db, read_k6_summary, read_file
tools_docs.go      stroppy://docs resource (embeds llms-full.txt via go:embed)
instructions.md    System prompt shipped to the assistant
```

## Dependencies

| Package | Purpose |
|---------|---------|
| [mcp-go](https://github.com/mark3labs/mcp-go) | Go MCP SDK — stdio transport, tool/resource protocol |
| [pgx](https://github.com/jackc/pgx) | PostgreSQL driver for `inspect_db` |

## Further reading

- **[SETUP.md](SETUP.md)** — end-to-end setup walkthrough with real benchmark results: stock vs tuned PostgreSQL config, TPC-B vs TPC-C scaling characteristics, connection pool sizing, concurrency sweep findings
- **[CLAUDE.md](CLAUDE.md)** — quick reference for the assistant (build, tools, architecture)
- **[instructions.md](instructions.md)** — system prompt that shapes how the assistant uses the tools
