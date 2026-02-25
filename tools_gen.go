package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var presets = map[string]string{
	"simple":      "Simple insert + query workload — good starting point for testing basic DB operations",
	"tpcb":        "TPC-B benchmark — simulates bank-like transactions (debit/credit between accounts)",
	"tpcc":        "TPC-C benchmark — complex OLTP workload simulating a wholesale supplier",
	"tpcds":       "TPC-DS benchmark — decision support / analytical queries",
	"execute_sql": "Raw SQL execution — run arbitrary SQL statements from a file",
}

// handleStroppyGen scaffolds a workspace using stroppy gen.
func handleStroppyGen(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	preset, err := request.RequireString("preset")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	workdir, err := request.RequireString("workdir")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if _, ok := presets[preset]; !ok {
		return mcp.NewToolResultError(fmt.Sprintf("unknown preset %q — valid presets: simple, tpcb, tpcc, tpcds, execute_sql", preset)), nil
	}

	args := []string{"gen", "--preset=" + preset, "--workdir=" + workdir}
	output, err := runStroppy(ctx, args, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stroppy gen failed: %v", err)), nil
	}

	// List generated files
	var files []string
	err = filepath.Walk(workdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(workdir, path)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		files = append(files, "(could not list files)")
	}

	result := fmt.Sprintf("Workspace generated at %s with preset %q\n\nGenerated files:\n- %s",
		workdir, preset, strings.Join(files, "\n- "))
	if output != "" {
		result = output + "\n\n" + result
	}

	return mcp.NewToolResultText(result), nil
}

// handleListPresets returns available workload presets.
func handleListPresets(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var sb strings.Builder
	sb.WriteString("Available Stroppy workload presets:\n\n")
	for name, desc := range presets {
		fmt.Fprintf(&sb, "- **%s**: %s\n", name, desc)
	}
	return mcp.NewToolResultText(sb.String()), nil
}
