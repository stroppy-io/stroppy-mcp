package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleInspectDB connects to a PostgreSQL database and returns version, config, and size.
func handleInspectDB(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := request.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("connection failed: %v", err)), nil
	}
	defer conn.Close(ctx)

	var sb strings.Builder

	// Version
	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get version: %v", err)), nil
	}
	fmt.Fprintf(&sb, "Version: %s\n\n", version)

	// Key tuning parameters
	sb.WriteString("Configuration:\n")
	params := []string{"shared_buffers", "max_connections", "work_mem", "effective_cache_size", "maintenance_work_mem"}
	for _, param := range params {
		var val string
		err := conn.QueryRow(ctx, "SHOW "+param).Scan(&val)
		if err != nil {
			val = fmt.Sprintf("(error: %v)", err)
		}
		fmt.Fprintf(&sb, "  %-25s %s\n", param+":", val)
	}

	// Database size
	var dbSize string
	err = conn.QueryRow(ctx, "SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize)
	if err != nil {
		dbSize = fmt.Sprintf("(error: %v)", err)
	}
	fmt.Fprintf(&sb, "\nDatabase size: %s\n", dbSize)

	// Current database name
	var dbName string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&dbName); err == nil {
		fmt.Fprintf(&sb, "Database name: %s\n", dbName)
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// k6Metric represents a single metric from k6 JSON output.
type k6Metric struct {
	Type     string             `json:"type"`
	Contains string             `json:"contains"`
	Values   map[string]float64 `json:"values"`
}

// handleReadK6Summary parses a k6 JSON summary file and formats key metrics.
func handleReadK6Summary(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := request.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	var raw struct {
		Metrics map[string]k6Metric `json:"metrics"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse JSON: %v", err)), nil
	}

	if raw.Metrics == nil {
		return mcp.NewToolResultError("no metrics found in JSON file"), nil
	}

	var sb strings.Builder
	sb.WriteString("K6 Test Results Summary\n")
	sb.WriteString("=======================\n\n")

	// Sort metric names for consistent output
	names := make([]string, 0, len(raw.Metrics))
	for name := range raw.Metrics {
		names = append(names, name)
	}
	sort.Strings(names)

	// Key duration metrics first
	keyMetrics := []string{
		"run_query_duration", "insert_duration",
		"http_req_duration", "iteration_duration",
	}
	for _, name := range keyMetrics {
		m, ok := raw.Metrics[name]
		if !ok || m.Type != "trend" {
			continue
		}
		fmt.Fprintf(&sb, "%s:\n", name)
		fmt.Fprintf(&sb, "  avg=%.2fms  min=%.2fms  max=%.2fms\n",
			m.Values["avg"], m.Values["min"], m.Values["max"])
		fmt.Fprintf(&sb, "  p(50)=%.2fms  p(90)=%.2fms  p(95)=%.2fms  p(99)=%.2fms\n",
			m.Values["med"], m.Values["p(90)"], m.Values["p(95)"], m.Values["p(99)"])
		sb.WriteString("\n")
	}

	// Counter/rate metrics
	for _, name := range names {
		m := raw.Metrics[name]
		switch m.Type {
		case "counter":
			fmt.Fprintf(&sb, "%-35s count=%.0f  rate=%.2f/s\n",
				name+":", m.Values["count"], m.Values["rate"])
		case "gauge":
			fmt.Fprintf(&sb, "%-35s value=%.0f  min=%.0f  max=%.0f\n",
				name+":", m.Values["value"], m.Values["min"], m.Values["max"])
		case "rate":
			passes := m.Values["passes"]
			fails := m.Values["fails"]
			total := passes + fails
			pct := float64(0)
			if total > 0 {
				pct = passes / total * 100
			}
			fmt.Fprintf(&sb, "%-35s %.2f%% (%.0f/%.0f)\n",
				name+":", pct, passes, total)
		}
	}

	// Remaining trend metrics not in key list
	keySet := make(map[string]bool)
	for _, k := range keyMetrics {
		keySet[k] = true
	}
	sb.WriteString("\nOther duration metrics:\n")
	for _, name := range names {
		m := raw.Metrics[name]
		if m.Type != "trend" || keySet[name] {
			continue
		}
		fmt.Fprintf(&sb, "  %-35s avg=%.2fms  p95=%.2fms\n",
			name+":", m.Values["avg"], m.Values["p(95)"])
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// handleReadFile reads a project file with size cap.
func handleReadFile(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := request.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid path: %v", err)), nil
	}

	// Check allowed extensions
	ext := strings.ToLower(filepath.Ext(absPath))
	allowed := map[string]bool{
		".ts": true, ".sql": true, ".yaml": true, ".yml": true,
		".json": true, ".toml": true, ".txt": true, ".md": true,
		".js": true, ".cfg": true, ".conf": true, ".env": true,
	}
	if !allowed[ext] {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported file extension %q — allowed: .ts, .sql, .yaml, .json, .toml, .txt, .md, .js", ext)), nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("file not found: %v", err)), nil
	}

	const maxSize = 100 * 1024 // 100KB
	if info.Size() > maxSize {
		return mcp.NewToolResultError(fmt.Sprintf("file too large: %d bytes (max %d)", info.Size(), maxSize)), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}
