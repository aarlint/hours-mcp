# Hours MCP - Professional Time Tracking & Invoice Generation

A comprehensive MCP (Model Context Protocol) server that enables contract-based time tracking and professional PDF invoice generation directly through Claude Desktop.

## 🚀 Quick Start

### 📦 Download & Install

1. **Download the binary** for your platform from [Latest Release](https://github.com/aarlint/hours-mcp/releases/latest):

   | Platform | Download |
   |----------|----------|
   | **macOS (Apple Silicon)** | `hours-mcp-darwin-arm64` |
   | **macOS (Intel)** | `hours-mcp-darwin-amd64` |
   | **Linux (x64)** | `hours-mcp-linux-amd64` |
   | **Linux (ARM64)** | `hours-mcp-linux-arm64` |
   | **Windows (x64)** | `hours-mcp-windows-amd64.exe` |

2. **Install the binary**:
   ```bash
   # Make executable and move to PATH
   chmod +x hours-mcp-*
   mv hours-mcp-* ~/.local/bin/hours-mcp
   ```

3. **Configure Claude Desktop**:

   Open Claude Desktop → Settings → Developer → Edit Config, then add:
   ```json
   {
     "mcpServers": {
       "hours": {
         "command": "/Users/YOUR_USERNAME/.local/bin/hours-mcp",
         "args": [],
         "env": {}
       }
     }
   }
   ```

   Replace `YOUR_USERNAME` with your actual username.

4. **Restart Claude Desktop** and start tracking time professionally! 🎯

### 🎯 First Steps

1. **Configure your business**: *"Set up my business information"*
2. **Add a client**: *"Add client Acme Corp with address 123 Main St, San Francisco, CA"*
3. **Create a contract**: *"Add contract AC-001 for Acme Corp at $150/hour"*
4. **Track time**: *"Add 2 hours for contract AC-001 today - API development"*
5. **Generate invoice**: *"Create invoice for Acme Corp for this month"*

## ✨ Features

- **Contract-Based Billing**: Professional contract management with individual rates and terms per client engagement
- **Client Management**: Add, edit, and manage clients with complete address information
- **Time Tracking**: Log hours in 15-minute increments against specific contracts with detailed descriptions
- **Invoice Generation**: Create professional PDF invoices with business branding, client addresses, and contract details
- **Payment Details**: Store and manage banking information per client
- **Recipient Management**: Add, list, and remove multiple recipient contacts for each client
- **Business Information**: Configure company details for professional invoice headers
- **Natural Language Input**: Add hours using commands like "add 2 hours for contract CA-001 today"
- **SQLite Storage**: All data stored locally with automatic migrations in `~/.hours/db`

## Database Schema

The system uses SQLite with the following entity relationships:

```mermaid
erDiagram
    clients {
        int id PK
        string name UK
        string address
        string city
        string state
        string zip_code
        string country
        datetime created_at
        datetime updated_at
    }

    contracts {
        int id PK
        int client_id FK
        string contract_number UK
        string name
        real hourly_rate
        string currency
        string contract_type
        date start_date
        date end_date
        string status
        string payment_terms
        string notes
        datetime created_at
        datetime updated_at
    }

    recipients {
        int id PK
        int client_id FK
        string name
        string email
        string title
        string phone
        boolean is_primary
        datetime created_at
    }

    payment_details {
        int id PK
        int client_id FK "UNIQUE"
        string bank_name
        string account_number
        string routing_number
        string swift_code
        string payment_terms
        string notes
        datetime updated_at
    }

    time_entries {
        string id PK "UUID"
        int client_id FK
        int contract_id FK
        date date
        real hours
        string description
        string contract_ref
        int invoice_id FK
        datetime created_at
    }

    invoices {
        int id PK
        int client_id FK
        string invoice_number UK
        date issue_date
        date due_date
        real total_amount
        string status
        string pdf_path
        datetime created_at
    }

    business_info {
        int id PK "Singleton"
        string business_name
        string contact_name
        string email
        string phone
        string address
        string city
        string state
        string zip_code
        string country
        string tax_id
        string website
        string logo_path
        string invoice_prefix
        datetime updated_at
    }

    migrations {
        int id PK
        string name UK
        datetime applied_at
    }

    clients ||--o{ contracts : "has contracts"
    clients ||--o{ recipients : "has contacts"
    clients ||--o| payment_details : "has payment info"
    clients ||--o{ invoices : "receives invoices"
    contracts ||--o{ time_entries : "tracks hours against"
    invoices ||--o{ time_entries : "includes entries"
    time_entries }o--|| contracts : "billed under"
    time_entries }o--|| clients : "worked for"
```

## Key Database Relationships

- **Clients** are the core entity with complete address information (rates moved to contracts)
- **Contracts** define billing relationships with specific rates, terms, and duration per client engagement
- **Recipients** are contact persons at each client organization (many-to-one with clients)
- **Payment Details** store banking and payment terms information (one-to-one with clients)
- **Time Entries** track billable hours with UUID identifiers linked to both contracts and clients
- **Invoices** group time entries for billing with automatic numbering (many-to-one with clients)
- **Business Info** is a singleton containing your company information for invoice headers
- **Migrations** track database schema changes for safe upgrades

### Contract-Based Architecture
The system uses a professional contract-based billing model where:
- Each client can have multiple contracts with different rates and terms
- Time entries are logged against specific contracts, not just clients
- Invoices are generated per contract, allowing separate billing for different engagements
- Historical rate changes are preserved through contract versioning

## 🛠️ Advanced Installation

### 📥 Pre-built Binaries (Recommended)

See the [Quick Start](#-quick-start) section above for binary downloads.

### 🔨 Build from Source

For developers who want to build from source:

#### Prerequisites
- Go 1.21 or later
- Claude Desktop app
- Git

#### Build Steps
```bash
# Clone the repository
git clone https://github.com/aarlint/hours-mcp.git
cd hours-mcp

# Download dependencies
go mod download

# Build for your platform
make build

# Or build for all platforms
make build-all

# Install to ~/.local/bin
make install
```

### ⚙️ Claude Desktop Configuration Details

#### Configuration File Location
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux**: `~/.config/Claude/claude_desktop_config.json`

#### Complete Configuration Example
```json
{
  "mcpServers": {
    "hours": {
      "command": "/Users/YOUR_USERNAME/.local/bin/hours-mcp",
      "args": [],
      "env": {}
    }
  }
}
```

#### Troubleshooting Configuration
- Replace `YOUR_USERNAME` with your actual system username
- Ensure the binary path is correct: `which hours-mcp`
- Verify binary is executable: `chmod +x ~/.local/bin/hours-mcp`
- Check Claude Desktop logs if connection fails
- Restart Claude Desktop after configuration changes

#### Verification
After restarting Claude Desktop, you should be able to use commands like:
- *"List all clients"*
- *"Set up my business information"*
- *"Add client Test Corp"*

## Usage Examples

### Client & Contract Management

```
"Add client Acme Corp with address 123 Business St, San Francisco, CA 94102"
"Edit client Acme Corp to update address to 456 New Street, Los Angeles, CA 90210"
"List all clients"
"Add contract AC-2025-001 for Acme Corp with rate $150/hour for Backend Development"
"List contracts for Acme Corp"
"Add recipient John Doe john@acmecorp.com for Acme Corp with title CTO"
"List recipients for Acme Corp"
"Remove recipient ID 5"
"Set payment details for Acme Corp: Bank of America, Net 30"
```

### Time Tracking

```
"Add 2 hours for contract AC-2025-001 today"
"Add 8 hours for contract AC-2025-001 this week"
"Add 4.5 hours for contract AC-2025-001 yesterday with description 'Backend API development'"
"List hours for this month"
"Show all hours for Acme Corp last week"
"Search time entries for contract AC-2025-001"
```

### Invoice Generation

```
"Create invoice for Acme Corp for this month"
"Make invoice for ClientX for last month"
"Create invoice for January 2025 for Acme Corp"
"List all pending invoices"
"Show invoice INV-202501-abc12345"
```

## Natural Language Time Entry

The MCP supports flexible natural language input:

- **Contract-based entries**: "Add 2 hours for contract CA-001 today"
- **Time periods**: "today", "yesterday", "this week", "last week", "this month"
- **Hour increments**: Supports 0.25 (15 min), 0.5 (30 min), 0.75 (45 min), etc.
- **Bulk entries**: "Add 8 hours for contract CA-001 this week" adds 8 hours to each weekday
- **Detailed descriptions**: All entries support rich descriptions for work performed

## Data Storage

All data is stored in SQLite at `~/.hours/db` with the following structure:
- Clients with complete address information
- Contracts with individual rates, terms, and status per client engagement
- Recipients for each client with management capabilities
- Payment details per client
- Time entries linked to both contracts and clients
- Generated invoices with PDF storage

## PDF Invoice Output

Invoices are generated as PDFs and saved to `~/Downloads/invoice_YYYY-MM-DD.pdf`

Each invoice includes:
- Business header with company information
- Client information and billing address
- Contract details (number, name, rate, terms)
- Itemized time entries with dates, descriptions, hours, and amounts
- Total hours and amount calculation
- Recipient contact information
- Payment details and banking information
- Due date (default: Net 30)

### Professional Features
- Single-contract billing for clean, focused invoices
- Automatic rate calculation from contract terms
- Complete audit trail with time entry UUIDs
- Professional formatting suitable for client delivery

## Development

```bash
# Run tests
make test

# Run locally for development
make run

# Clean build artifacts
make clean
```

## License

MIT