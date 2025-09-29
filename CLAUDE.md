# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Development Commands

### Building and Installation
```bash
# Build for Apple Silicon
make build
# or manually:
GOOS=darwin GOARCH=arm64 go build -o hours-mcp main.go

# Install to ~/.local/bin
make install

# Build and install to custom location
GOOS=darwin GOARCH=arm64 go build -o /Users/austin/.local/bin/hours-mcp main.go
```

### Development and Testing
```bash
# Run locally for development/testing
make run
# or:
go run main.go

# Run tests
make test
# or:
go test ./...

# Download/update dependencies
make deps
# or:
go mod download && go mod tidy

# Clean build artifacts
make clean
```

## Architecture Overview

This is an MCP (Model Context Protocol) server for time tracking and invoice generation, built in Go for Apple Silicon Macs.

### Core Components

**MCP Server Foundation**
- Built on `github.com/modelcontextprotocol/go-sdk/mcp`
- Exposes tools through MCP protocol for Claude Desktop integration
- All client interaction happens through MCP tool calls, not direct CLI commands

**Database Layer** (`internal/database/`)
- SQLite database stored at `~/.hours/db`
- Schema includes: clients, recipients, payment_details, time_entries, invoices
- Foreign key relationships with CASCADE deletes for data integrity

**Tool Registration** (`internal/server/register.go`)
- Central hub where all MCP tools are registered
- Contains business logic for each tool handler
- Handles database transactions and error management
- Tools include: client management, time tracking, invoice generation, bulk operations, search/delete functionality

**Data Models** (`internal/models/`)
- Go structs representing database entities
- JSON serialization for MCP responses
- Proper null handling for optional fields (e.g., `InvoiceID *int`)

**Time Parsing** (`internal/timeparse/`)
- Natural language date parsing ("today", "yesterday", "this week")
- Period parsing for invoice generation ("this month", "January 2025")
- Supports both absolute dates (YYYY-MM-DD) and relative expressions

**PDF Generation** (`internal/pdf/`)
- Uses `github.com/johnfercher/maroto/v2` for PDF creation
- Generates professional invoices saved to ~/Downloads
- Includes client info, itemized time entries, payment details

### Key Design Patterns

**Handler Pattern**: `server.Handler` struct with `*sql.DB` provides database access methods like `getClientIDByName()`

**Transaction Management**: Bulk operations and invoice creation use database transactions for atomicity

**Error Handling**: All MCP tools return structured errors that Claude Desktop can display to users

**Type Safety**: Extensive use of proper Go types with JSON schema annotations for MCP tool arguments

### Tool Categories

**Core Operations**: add_client, add_hours, list_hours, create_invoice
**Bulk Operations**: bulk_add_hours, bulk_delete_time_entries
**Management**: delete_time_entry, update_time_entry, get_time_entry_details
**Search/Filter**: search_time_entries with multiple filter criteria
**Utility**: list_clients, add_recipient, set_payment_details

### Data Flow

1. Claude Desktop calls MCP tool with JSON arguments
2. Tool handler validates arguments and converts to Go types
3. Database operations performed (often in transactions)
4. Results formatted as MCP response with both human-readable text and structured data
5. Response returned to Claude Desktop for user display

### Database Schema Notes

- `time_entries.invoice_id` links to invoices (NULL = unbilled)
- `payment_details` has UNIQUE constraint on client_id (one per client)
- Indexes on commonly queried fields (date, client_id, status)
- All timestamps use DATETIME DEFAULT CURRENT_TIMESTAMP

### Development Notes

- Built for Apple Silicon specifically (GOOS=darwin GOARCH=arm64)
- SQLite driver requires CGO (uses `github.com/mattn/go-sqlite3`)
- MCP protocol requires stdio transport for Claude Desktop integration
- PDF generation depends on several image/PDF processing libraries
- after making changes build and push to local bin /Users/austin/.local/bin/hours-mcp