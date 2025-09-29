package main

import (
	"context"
	"fmt"
	"os"

	"github.com/austin/hours-mcp/internal/database"
	"github.com/austin/hours-mcp/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// version is set by build-time ldflags
var version = "dev"

func main() {
	// Handle version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("hours-mcp version %s\n", version)
		os.Exit(0)
	}

	// Initialize database
	db, err := database.Initialize()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "hours-mcp",
		Version: version,
	}, nil)

	// Register tools with the server
	server.RegisterTools(mcpServer, db)

	// Run the server on stdio transport
	if err := mcpServer.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}