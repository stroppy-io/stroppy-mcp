# stroppy-mcp: Setup Guide

## The Big Picture

Stroppy-mcp is a thin bridge: it takes tool calls from an AI assistant, translates them into `stroppy` CLI invocations, and returns structured results. The moving parts are:

```
AI assistant  -->  stroppy-mcp (Go binary, MCP stdio)  -->  stroppy CLI  -->  PostgreSQL
```

Three things must exist on the machine before any of this works:

| Dependency | What it is | Why it's needed |
|---|---|---|
| **PostgreSQL** | The database under test | Stroppy has nothing to stress without it |
| **stroppy** | CLI binary (a custom k6 build with database extensions) | The actual test runner — generates data, executes workloads, collects metrics |
| **stroppy-mcp** | This project — a Go binary that speaks MCP over stdio | Gives the AI assistant tool-level access to stroppy instead of raw shell commands |

That's it. No containers, no Kubernetes, no cloud accounts. Everything runs locally.

## Obtaining the Dependencies

### PostgreSQL

Any PostgreSQL 14+ will do. How you install it doesn't matter as long as Stroppy can reach it over TCP with a connection URL.

| Method | Command | Notes |
|---|---|---|
| Ubuntu/Debian apt | `sudo apt install postgresql` | Runs as `postgres` system user, TCP on 5432 out of the box |
| Ubuntu snap | `sudo snap install postgresql` | Connects locally via `psql -U postgres -h /tmp`, TCP needs a password set |
| Homebrew (macOS) | `brew install postgresql@16` | `brew services start postgresql@16` |
| Docker | `docker run -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:16` | Zero setup, disposable |
| Already running | - | Just need the connection URL |

**The only thing Stroppy needs:** a `postgres://user:pass@host:port/db?sslmode=disable` URL that works. If you can `psql` into it with that URL, Stroppy can too.

**Common gotcha:** fresh installs often don't have a password set for TCP auth. Connect however your install allows (unix socket, peer auth, etc.) and run `ALTER USER postgres PASSWORD 'postgres';` once.

**Docker without sudo:** if you use Docker (for the cleanroom or for PostgreSQL), your user must be in the `docker` group — otherwise every `docker` command requires `sudo`, which means the AI assistant will prompt for permission on every invocation. Fix it once:

```bash
sudo usermod -aG docker $USER
newgrp docker    # or log out and back in
```

Also: avoid `snap install docker` — it doesn't create a `docker` group and the socket permissions are awkward. Use the [official Docker apt repository](https://docs.docker.com/engine/install/ubuntu/) instead.

### Stroppy CLI

The stroppy binary is a self-contained executable with no runtime dependencies. Two ways to get it:

**Download a release (seconds):**

```bash
curl -L https://github.com/stroppy-io/stroppy/releases/latest/download/stroppy_linux_amd64.tar.gz | tar xz
sudo mv stroppy /usr/local/bin/
```

Releases exist for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64. Replace the platform suffix as needed.

**Build from source (minutes, needs Go 1.24+ and xk6):**

```bash
go install go.k6.io/xk6/cmd/xk6@latest
cd stroppy && make build    # produces ./build/stroppy
```

Only do this if you're developing Stroppy itself.

### stroppy-mcp

```bash
cd stroppy-mcp && go build -o stroppy-mcp .
```

## Wiring It Together

The MCP server finds the stroppy binary in this order:

1. `STROPPY_BIN` environment variable
2. `./build/stroppy` relative to CWD
3. `stroppy` on `$PATH`

Configure in Claude Code settings (`~/.claude/settings.json` or project `.mcp.json`):

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

Setting `STROPPY_BIN` explicitly is the safest option — it removes any ambiguity about working directory.

## Example: Config Inspection vs Benchmark Reality

This section was generated from a real session — a fresh snap PostgreSQL 16.11 on a 16-core / 64GB RAM VM with a 200GB virtual disk (NVMe 4.0 backed).

### What `inspect_db` revealed

```
shared_buffers:        128MB
work_mem:              4MB
effective_cache_size:  4GB
maintenance_work_mem:  64MB
max_connections:       100
wal_buffers:           4MB
max_wal_size:          1GB
effective_io_concurrency: 1
random_page_cost:      4.0
```

This is a stock PostgreSQL config — tuned for a machine with 512MB of RAM and a spinning disk. On a 64GB box with NVMe 4.0 storage it's leaving ~98% of memory on the table and assuming IO characteristics three orders of magnitude slower than reality.

### What the benchmarks showed

Four runs, all 30 seconds, scale factor 1, stock PostgreSQL config:

#### TPC-B (simple bank transactions: debit/credit between accounts)

| | 10 VUs | 100 VUs | Delta |
|---|---|---|---|
| **Throughput** | 4,856 q/s | 5,282 q/s | +8.7% |
| **Avg latency** | 1.5ms | 18.2ms | **12x worse** |
| **p90 latency** | 4ms | 21ms | 5x worse |
| **p95 latency** | 5ms | 23ms | 4.6x worse |
| **Max latency** | 47ms | 60ms | +28% |
| **Errors** | 0 | 0 | - |

Throughput barely moved when going 10x on concurrency. The database hit a ceiling around 5,300 q/s while latency exploded — classic sign of lock contention on the `pgbench_branches` row (TPC-B with scale factor 1 has a single branch, so every transaction touches the same row). Adding VUs just piles up waiters.

#### TPC-C (complex OLTP: new_order, payment, order_status, delivery, stock_level)

| | 11 VUs | 99 VUs | Delta |
|---|---|---|---|
| **Throughput** | 9,737 q/s | 18,252 q/s | **+87%** |
| **Avg latency** | 0.63ms | 4.8ms | 7.6x worse |
| **p90 latency** | 2ms | 2ms | same |
| **p95 latency** | 3ms | 6ms | 2x worse |
| **Max latency** | 31ms | 17,920ms | **578x worse** |
| **Errors** | 0 | 0 | - |

TPC-C scaled much better — nearly doubled throughput at 9x concurrency. The workload spreads across 5 different transaction types and multiple tables, so there's far less single-row contention. But that 17.9-second max latency at 99 VUs is a red flag: with `shared_buffers=128MB` and 100K items + 100K stock rows, the buffer pool is likely thrashing. A few unlucky transactions got stuck waiting for IO while buffers were evicted and reloaded.

#### TPC-B vs TPC-C side by side

| | TPC-B 10 VUs | TPC-C 11 VUs | TPC-C 99 VUs |
|---|---|---|---|
| **Throughput** | 4,856 q/s | 9,737 q/s | 18,252 q/s |
| **Avg latency** | 1.5ms | 0.63ms | 4.8ms |
| **p95 latency** | 5ms | 3ms | 6ms |

TPC-C at low concurrency is 2x faster than TPC-B despite being a much more complex workload. Why? TPC-B's bottleneck is the single branch row — every transaction does `UPDATE pgbench_branches SET bbalance = bbalance + $1 WHERE bid = $2`, and with scale factor 1 there's exactly one branch. TPC-C distributes load across warehouses, districts, and customers, so the lock graph is much wider.

#### What this tells us about the stock config

The numbers look fine at first glance, but they're masking problems:

1. **The entire dataset fits in OS page cache** — with 64GB of RAM and ~38MB of data, these benchmarks never actually hit disk. The `shared_buffers=128MB` setting isn't hurting yet, but it will the moment the dataset grows past what the OS caches.

2. **The 17.9s max in TPC-C** is the canary — even with everything in memory, 99 concurrent connections with a 128MB buffer pool causes enough internal contention (buffer eviction, WAL flushes, lock waits) to occasionally stall a transaction for 18 seconds.

3. **TPC-B's 5,300 q/s ceiling** has nothing to do with hardware — a single-row hot spot will bottleneck at the same throughput regardless of CPU count or memory. Increasing `scale_factor` (more branches) would help, but so would the tuning below.

The config is lying to the planner in several ways: `effective_cache_size=4GB` on a 64GB machine means the optimizer underestimates what's cached and may choose sequential scans over index scans on larger datasets. `random_page_cost=4` is a spinning-disk assumption. `effective_io_concurrency=1` means no read-ahead.

### What we changed

Based on the hardware (64GB RAM, 16 cores, NVMe 4.0 storage):

| Parameter | Stock | Tuned | Why |
|---|---|---|---|
| `shared_buffers` | 128MB | 16GB | ~25% of RAM is the standard starting point |
| `effective_cache_size` | 4GB | 48GB | ~75% of RAM — tells the planner what the OS will cache |
| `work_mem` | 4MB | 64MB | With 100 max connections: 100 * 64MB = 6.4GB worst case, safe on 64GB |
| `maintenance_work_mem` | 64MB | 2GB | Speeds up VACUUM, CREATE INDEX, data loading |
| `wal_buffers` | 4MB | 64MB | Reduces WAL write contention under concurrent writes |
| `max_wal_size` | 1GB | 4GB | Fewer checkpoint flushes during sustained write bursts |
| `random_page_cost` | 4.0 | 1.1 | NVMe — random reads are nearly as fast as sequential |
| `effective_io_concurrency` | 1 | 200 | NVMe 4.0 can handle hundreds of concurrent reads |

These are not exotic tweaks — they're the standard PGTune recommendations for this hardware class.

### After tuning: stock vs tuned, side by side

All runs: 30 seconds, scale factor 1, same machine.

#### TPC-B

| | Stock 10 VUs | Tuned 10 VUs | Stock 100 VUs | Tuned 100 VUs |
|---|---|---|---|---|
| **Throughput** | 4,856 q/s | 4,750 q/s | 5,282 q/s | 4,921 q/s |
| **Avg latency** | 1.5ms | 1.5ms | 18.2ms | 19.5ms |
| **p95 latency** | 5ms | 5ms | 23ms | 26ms |
| **Max latency** | 47ms | 48ms | 60ms | 79ms |

#### TPC-C

| | Stock 11 VUs | Tuned 11 VUs | Stock 99 VUs | Tuned 99 VUs |
|---|---|---|---|---|
| **Throughput** | 9,737 q/s | 9,525 q/s | 18,252 q/s | 17,490 q/s |
| **Avg latency** | 0.63ms | 0.66ms | 4.8ms | 5.0ms |
| **p95 latency** | 3ms | 3ms | 6ms | 6ms |
| **Max latency** | 31ms | 48ms | **17,920ms** | **12,340ms** |

#### What the tuning did (and didn't do)

**Throughput and average latency: no change.** Within noise margin across all four scenarios. This is expected — with scale factor 1, the entire dataset is ~38MB. On a 64GB machine it lives entirely in OS page cache regardless of whether `shared_buffers` is 128MB or 16GB. The tuning parameters target a bottleneck that doesn't exist yet at this scale.

**Max latency under high concurrency: improved.** TPC-C at 99 VUs dropped from 17.9s to 12.3s worst-case — a 31% improvement. The larger buffer pool reduced the chance of unlucky transactions getting stuck on buffer eviction storms. Still bad, but measurably less bad.

**Where tuning would matter:** a larger dataset (scale factor 10-100+, hundreds of MB to GB) that exceeds OS page cache, or sustained write-heavy workloads where WAL checkpoint flushes and `maintenance_work_mem` come into play. The planner-hint parameters (`effective_cache_size`, `random_page_cost`, `effective_io_concurrency`) affect query plan choices that only diverge when the optimizer is choosing between sequential and index scans on large tables.

**The takeaway for this doc:** tuning config on a toy dataset proves nothing either way. But the *workflow* — inspect, benchmark, tune, re-benchmark, compare — is exactly what stroppy-mcp enables. The value isn't in any single run; it's in making the iteration loop fast enough that you actually do it.

### Scaling concurrency: connection pool, VUs, and warehouses

The tuning section above changed PostgreSQL config. This section explores the other side — how test parameters affect throughput. The machine is the same (16-core, 64GB, NVMe 4.0) with the tuned config (`shared_buffers=16GB`, `max_connections=400`).

The TPC-C script has three knobs that matter:

| Knob | What it controls | Default |
|---|---|---|
| `VUS_SCALE` | Multiplier on virtual users across all 5 scenarios. `1` = 99 VUs, `2` = 198, etc. | 1 |
| `WAREHOUSES` | Number of TPC-C warehouses (data volume and contention spread) | 1 |
| `POOL_SIZE` | Shared connection pool size in the driver | 100 |

A systematic sweep across these parameters reveals sharp boundaries.

#### The connection pool cliff

The single biggest factor in TPC-C throughput is not lock contention, buffer pool size, or warehouse count — it's whether the connection pool is large enough for the VU count.

| VUS_SCALE | VUs | Pool Size | TPS | Avg Latency |
|---|---|---|---|---|
| 2 | 198 | 100 | **1,346** | 137ms |
| 2 | 198 | 200 | **19,751** | 8.9ms |

Same workload, same database, same hardware. The only difference is pool size. With 198 VUs competing for 100 connections, most VUs spend their time waiting for a connection rather than executing queries. TPS drops by **14.7x** and average latency goes from 9ms to 137ms.

**Rule of thumb:** set `POOL_SIZE` >= total VU count. The script's `sharedConnections` in its driver config controls this — pass it via the `POOL_SIZE` env var if the script supports it, or edit the script directly.

#### VU scaling at small dataset sizes

With the pool sized correctly and `WAREHOUSES=1` (entire dataset fits in memory at ~38MB):

| VUS_SCALE | VUs | Pool | TPS | Avg Latency | p95 Latency |
|---|---|---|---|---|---|
| 0.5 | 50 | 100 | 10,138 | 4.2ms | 9ms |
| 0.75 | 74 | 100 | 13,438 | 4.8ms | 8ms |
| 1 | 99 | 100 | 17,960 | 4.9ms | 6ms |
| 2 | 198 | 200 | 19,751 | 8.9ms | 6ms |
| 3 | 297 | 300 | 22,728 | 11.7ms | 8ms |
| 4 | 396 | 400 | 24,034 | 14.8ms | 8ms |

TPS scales almost linearly up to `VUS_SCALE=3`, then starts to flatten. The ceiling is a combination of `max_connections=400` and the fact that even with distributed TPC-C transactions, there's intra-district locking at high concurrency. Latency rises steadily — the classic concurrency-throughput-latency tradeoff.

#### Warehouse count at fixed concurrency

With `VUS_SCALE=2` (198 VUs) and `POOL_SIZE=200`:

| Warehouses | Setup Time | Reported TPS | Steady-State TPS* | Avg Latency |
|---|---|---|---|---|
| 1 | ~2s | 19,751 | ~21,255 | 8.9ms |
| 2 | ~4s | 19,502 | ~21,880 | 8.6ms |
| 4 | ~7s | 18,474 | ~22,580 | 8.4ms |
| 8 | ~12s | 18,347 | ~25,452 | 7.4ms |

*Steady-state TPS = total iterations / 30s workload duration, excluding setup.

The reported TPS numbers (which k6 computes as total queries / total elapsed time including setup) decrease with more warehouses because data loading takes longer. But steady-state throughput during the 30-second workload window actually *increases* — more warehouses spread contention across more districts, reducing lock waits. The latency improvement at 8 warehouses (7.4ms vs 8.9ms at 1 warehouse) confirms this.

**Caveat:** at scale factor 1, each warehouse adds ~38MB. Even at 8 warehouses the whole dataset (~300MB) fits comfortably in the 16GB buffer pool. The warehouse count will matter much more at larger scale factors where data exceeds memory.

#### Combinations that exceed 15,000 TPS

| Warehouses | VUS_SCALE | VUs | Pool | Reported TPS |
|---|---|---|---|---|
| 1 | 1 | 99 | 100 | 17,960 |
| 1 | 2 | 198 | 200 | 19,751 |
| 1 | 3 | 297 | 300 | 22,728 |
| 1 | 4 | 396 | 400 | 24,034 |
| 2 | 1 | 99 | 100 | 17,074 |
| 2 | 2 | 198 | 200 | 19,502 |
| 4 | 2 | 198 | 200 | 18,474 |
| 8 | 2 | 198 | 200 | 18,347 |

The pattern: at small scale factors, **VU count is the primary driver of throughput** (given adequate pool size). Warehouse count has a secondary effect through contention reduction but the dominant factor is raw parallelism. The minimum to reliably exceed 15k TPS on this hardware is approximately `VUS_SCALE=1` (99 VUs) with `POOL_SIZE >= VUs`.

#### Combinations that do not reach 15,000 TPS

| Warehouses | VUS_SCALE | VUs | Pool | TPS | Why |
|---|---|---|---|---|---|
| 1 | 0.5 | 50 | 100 | 10,138 | Too few VUs to saturate the database |
| 1 | 0.75 | 74 | 100 | 13,438 | Still below parallelism threshold |
| 1 | 2 | 198 | 100 | 1,346 | Pool starvation — 198 VUs, 100 connections |

### The workflow this enables

```
User: "benchmark my fresh postgres install"
  --> inspect_db (see the stock config)
  --> stroppy_run tpcb (get baseline numbers)
  --> assistant spots the mismatch between hardware and config
  --> suggests tuning changes with reasoning

User: "ok apply those and re-run"
  --> ALTER SYSTEM SET shared_buffers = '16GB'; ... SELECT pg_reload_conf();
  --> stroppy_run tpcb again
  --> compare: did throughput go up? did p95 drop?

User: "now try with a bigger dataset"
  --> stroppy_run with SCALE_FACTOR=10
  --> this is where tuning starts to show its teeth
```

This inspect-benchmark-suggest-iterate loop is the core value of having stroppy accessible through MCP. The assistant doesn't just run a command — it reads the config, understands the hardware context, runs a real workload, and connects the dots between configuration and observed performance.

## Where MCP Adds Value (and Where It Doesn't)

Stroppy is a CLI tool. Everything it does can be done with bash commands. The question is: when does wrapping it in MCP actually help?

### MCP shines: exploratory and iterative work

**User:** "I just set up Postgres, stress test it and show me the results"

Without MCP, the assistant would need to: guess the connection URL, figure out sslmode, run stroppy gen, understand the generated file layout, construct the right `stroppy run` command with env vars, parse the output, and explain the metrics. Each step is a bash call that might fail, requiring back-and-forth debugging.

With MCP, the assistant calls `inspect_db` (confirms connectivity, gets version and tuning params), `stroppy_gen` (scaffolds a workspace), `stroppy_run` (executes and gets structured metrics back). Three tool calls, each with clear success/failure semantics.

**User:** "Run TPC-B with 50 VUs for 5 minutes, then bump to 100 VUs and compare"

The assistant can issue two `stroppy_run` calls with different parameters and compare the returned metrics directly — no output parsing, no grep-ing for p95 values, no mental model of what k6 stdout looks like.

**User:** "Sweep VUS_SCALE and WAREHOUSES to find which combinations exceed 15k TPS"

This is where MCP's value is most obvious. A parameter sweep means 10-15 sequential benchmark runs, each with different `env` values. Via bash, that's 10-15 unique commands, each requiring a permission approval. Via MCP, the assistant calls `stroppy_run` repeatedly with different `env` strings — no approvals, no command construction errors, no output parsing. The assistant can track results across runs, spot the connection pool cliff, and pivot its exploration strategy based on intermediate numbers. In the session that produced the concurrency scaling section above, 13 runs were executed in a single uninterrupted conversation.

**User:** "What's the current state of my database?"

`inspect_db` returns version, shared_buffers, max_connections, work_mem, effective_cache_size, and database size in a single call. The bash equivalent is 6 separate SQL queries or remembering the right `SHOW` commands.

**User:** "Check if my test script has errors before running it"

`stroppy_validate` does a dry-run transpile check. Without it, the assistant would run the test and parse error output from k6's stderr.

### The synergy: MCP tools + general assistant capabilities

MCP tools don't work in isolation — they pull the assistant into a workflow where its other capabilities (reading code, editing files, reasoning about results) become part of the same loop.

During the concurrency sweep that produced the scaling section above, the assistant hit a wall: 198 VUs with the default script produced 1,346 TPS instead of the expected ~20k. The user pointed out that the connection pool size (hardcoded at 100 in the generated script) was likely the bottleneck. The assistant read the TypeScript source with `read_file`, found the `sharedConnections: 100` line, edited the script to make pool size configurable via a `POOL_SIZE` env var, and re-ran. TPS jumped to 19,751. Same session, no context switch.

None of those individual steps are remarkable — reading a file, editing a line, re-running a test. But without MCP in the loop, the assistant wouldn't have been running the benchmarks in the first place. MCP creates the context where the assistant has a reason to inspect the script, understands what the numbers mean, and can act on the diagnosis immediately. The tool calls and the file edits aren't separate workflows — they're one conversation.

Similarly, the generated TPC-C script had VU counts hardcoded (44/43/4/4/4). The assistant added `VUS_SCALE` support by wrapping them in `Math.max(1, Math.round(N * VUS_SCALE))`, then used `stroppy_run` with `env="VUS_SCALE=3"` to verify it worked. Scaffold, discover a limitation, fix it, validate — all within the same iterative loop that MCP enables.

### MCP doesn't add much: one-shot runs by experienced users

If you already know your connection URL, your script path, and the k6 flags you want — a direct bash command is simpler:

```bash
DRIVER_URL="postgres://..." DURATION="5m" stroppy run tpcb.ts tpcb.sql -- --vus 50
```

The MCP layer adds value when the assistant needs to **discover, iterate, or react to results** — not when the human already knows exactly what to run.

### The permission problem (and why MCP solves it)

When an AI assistant runs stroppy via bash, every invocation looks different:

```bash
K6_WEB_DASHBOARD=true K6_WEB_DASHBOARD_EXPORT=report-tpcb-10vu.html \
  DRIVER_URL="postgres://..." DURATION="30s" VUS=10 \
  ./stroppy run tpcb.ts tpcb.sql
```

```bash
K6_WEB_DASHBOARD=true K6_WEB_DASHBOARD_EXPORT=report-tpcc-100vu-tuned.html \
  DRIVER_URL="postgres://..." DURATION="30s" VUS_SCALE=1 \
  ./stroppy run tpcc.ts tpcc.sql
```

Each combination of env vars, paths, and flags produces a unique command string. Claude Code's permission system matches on the full command, so the user must approve every single run. In the session that produced this doc, that meant 8 manual approvals — one per benchmark run — breaking the user's flow each time.

MCP tool calls don't have this problem. The `stroppy_run` tool is approved once (via the MCP server config), and then all invocations go through without interruption — regardless of parameters. The assistant calls:

```
stroppy_run(script="tpcb.ts", sql_file="tpcb.sql", vus="10", duration="30s", report_path="report.html")
```

No bash, no env vars, no permission prompts. The MCP server handles `DRIVER_URL`, `DURATION`, `VUS`, `K6_WEB_DASHBOARD`, and `K6_WEB_DASHBOARD_EXPORT` internally.

The `stroppy_run` tool now supports:
- `vus` — passed as `VUS` env var (scripts should read `__ENV.VUS`)
- `duration` — passed as `DURATION` env var (scripts should read `__ENV.DURATION`)
- `driver_url` — passed as `DRIVER_URL` env var
- `report_path` — enables `K6_WEB_DASHBOARD` and exports the HTML report to this path

This means the entire benchmark workflow — scaffold, run, tune, re-run, compare — can happen without a single bash permission prompt.
