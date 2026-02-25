package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleStroppyRun executes a stress test via stroppy run.
func handleStroppyRun(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	script, err := request.RequireString("script")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	sqlFile := request.GetString("sql_file", "")
	envStr := request.GetString("env", "")
	duration := request.GetString("duration", "")
	driverURL := request.GetString("driver_url", "")
	extraArgs := request.GetString("extra_args", "")
	reportPath := request.GetString("report_path", "")

	args := []string{"run", script}
	if sqlFile != "" {
		args = append(args, sqlFile)
	}

	// Build k6 passthrough args after "--"
	var k6Args []string
	if extraArgs != "" {
		k6Args = append(k6Args, strings.Fields(extraArgs)...)
	}
	if len(k6Args) > 0 {
		args = append(args, "--")
		args = append(args, k6Args...)
	}

	// Parse env: space-separated KEY=VALUE pairs from the caller,
	// plus first-class parameters (driver_url, duration, report_path).
	env := map[string]string{}
	if envStr != "" {
		for _, pair := range strings.Fields(envStr) {
			if k, v, ok := strings.Cut(pair, "="); ok {
				env[k] = v
			}
		}
	}
	if driverURL != "" {
		env["DRIVER_URL"] = driverURL
	}
	if duration != "" {
		env["DURATION"] = duration
	}
	if reportPath != "" {
		env["K6_WEB_DASHBOARD"] = "true"
		env["K6_WEB_DASHBOARD_EXPORT"] = reportPath
	}

	output, err := runStroppy(ctx, args, env)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stroppy run failed: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleStroppyValidate does a dry-run syntax check on a script.
func handleStroppyValidate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	script, err := request.RequireString("script")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	sqlFile := request.GetString("sql_file", "")

	args := []string{"run", script}
	if sqlFile != "" {
		args = append(args, sqlFile)
	}
	args = append(args, "--", "--iterations", "0", "--duration", "1s")

	output, err := runStroppy(ctx, args, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("validation failed: %v", err)), nil
	}

	return mcp.NewToolResultText("Validation passed.\n\n" + output), nil
}
