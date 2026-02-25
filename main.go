package main

import (
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"stroppy-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithInstructions(instructions),
	)

	// --- Tools ---

	s.AddTool(
		mcp.NewTool("stroppy_gen",
			mcp.WithDescription("Scaffold a Stroppy workspace from a preset. Creates test script (.ts), SQL file, and config in the target directory."),
			mcp.WithString("preset",
				mcp.Required(),
				mcp.Description("Workload preset to generate"),
				mcp.Enum("simple", "tpcb", "tpcc", "tpcds", "execute_sql"),
			),
			mcp.WithString("workdir",
				mcp.Required(),
				mcp.Description("Target directory for the generated workspace"),
			),
		),
		handleStroppyGen,
	)

	s.AddTool(
		mcp.NewTool("stroppy_run",
			mcp.WithDescription("Execute a Stroppy stress test. Runs the given TypeScript script against a database using k6. Returns the end-of-run summary with metrics."),
			mcp.WithString("script",
				mcp.Required(),
				mcp.Description("Path to the .ts test script"),
			),
			mcp.WithString("sql_file",
				mcp.Description("Path to .sql file (required for some workloads)"),
			),
			mcp.WithString("env",
				mcp.Description("Environment variables for the script as KEY=VALUE pairs separated by spaces, e.g. 'VUS_SCALE=5 WAREHOUSES=10'. Each script defines its own knobs — read the script to discover them."),
			),
			mcp.WithString("duration",
				mcp.Description("Test duration (passed as DURATION env var to the script), e.g. '30s', '5m'"),
			),
			mcp.WithString("driver_url",
				mcp.Description("Database connection URL (sets DRIVER_URL env var), e.g. 'postgres://user:pass@localhost:5432/db'"),
			),
			mcp.WithString("report_path",
				mcp.Description("Path to save an HTML report. Enables the k6 web dashboard and exports it to this file on completion."),
			),
			mcp.WithString("extra_args",
				mcp.Description("Additional k6 CLI arguments as a single string, e.g. '--iterations 100 --no-teardown'"),
			),
		),
		handleStroppyRun,
	)

	s.AddTool(
		mcp.NewTool("stroppy_validate",
			mcp.WithDescription("Dry-run a Stroppy script to check for transpile/parse errors without actually running the test."),
			mcp.WithString("script",
				mcp.Required(),
				mcp.Description("Path to the .ts test script to validate"),
			),
			mcp.WithString("sql_file",
				mcp.Description("Path to .sql file (if the script requires one)"),
			),
		),
		handleStroppyValidate,
	)

	s.AddTool(
		mcp.NewTool("read_k6_summary",
			mcp.WithDescription("Parse a k6 JSON summary file and return formatted metrics: latency percentiles, throughput, error rates."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the k6 JSON summary file"),
			),
		),
		handleReadK6Summary,
	)

	s.AddTool(
		mcp.NewTool("inspect_db",
			mcp.WithDescription("Connect to a PostgreSQL database and return version, tuning parameters (shared_buffers, max_connections, work_mem), and database size."),
			mcp.WithString("url",
				mcp.Required(),
				mcp.Description("PostgreSQL connection URL, e.g. 'postgres://user:pass@localhost:5432/db'"),
			),
		),
		handleInspectDB,
	)

	s.AddTool(
		mcp.NewTool("list_presets",
			mcp.WithDescription("List all available Stroppy workload presets with descriptions."),
		),
		handleListPresets,
	)

	s.AddTool(
		mcp.NewTool("read_file",
			mcp.WithDescription("Read a project file (.ts, .sql, .yaml, .json, .toml, .txt, .md, .js). Capped at 100KB."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the file to read"),
			),
		),
		handleReadFile,
	)

	// --- Resources ---

	s.AddResource(
		mcp.NewResource(
			"stroppy://docs",
			"Stroppy Documentation",
			mcp.WithResourceDescription("Full Stroppy documentation — API reference, SQL syntax, generators, drivers, workloads, and configuration."),
			mcp.WithMIMEType("text/plain"),
		),
		handleReadDocs,
	)

	// --- Serve ---

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
