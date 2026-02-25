package main

import (
	"context"
	_ "embed"

	"github.com/mark3labs/mcp-go/mcp"
)

//go:embed llms-full.txt
var stroppyDocs string

//go:embed instructions.md
var instructions string

// handleReadDocs returns the full Stroppy documentation.
func handleReadDocs(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "stroppy://docs",
			MIMEType: "text/plain",
			Text:     stroppyDocs,
		},
	}, nil
}
