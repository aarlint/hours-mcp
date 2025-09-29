package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/austin/hours-mcp/internal/models"
	"github.com/austin/hours-mcp/internal/pdf"
	"github.com/austin/hours-mcp/internal/timeparse"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers all tools with the MCP server
func RegisterTools(server *mcp.Server, db *sql.DB) {
	h := &Handler{db: db}

	// Add Client tool
	type addClientArgs struct {
		Name    string `json:"name" jsonschema:"Client name"`
		Address string `json:"address,omitempty" jsonschema:"Street address"`
		City    string `json:"city,omitempty" jsonschema:"City"`
		State   string `json:"state,omitempty" jsonschema:"State or province"`
		ZipCode string `json:"zip_code,omitempty" jsonschema:"ZIP or postal code"`
		Country string `json:"country,omitempty" jsonschema:"Country"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_client",
		Description: "Add a new client (note: rates are now managed through contracts)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addClientArgs) (*mcp.CallToolResult, any, error) {
		result, err := db.Exec(`
			INSERT INTO clients (name, address, city, state, zip_code, country)
			VALUES (?, ?, ?, ?, ?, ?)
		`, args.Name, args.Address, args.City, args.State, args.ZipCode, args.Country)

		if err != nil {
			return nil, nil, fmt.Errorf("failed to add client: %w", err)
		}

		id, _ := result.LastInsertId()

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Client '%s' added successfully (ID: %d)", args.Name, id),
				},
			},
		}, nil, nil
	})

	// List Clients tool
	type listClientsArgs struct{}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_clients",
		Description: "List all clients",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listClientsArgs) (*mcp.CallToolResult, any, error) {
		rows, err := db.Query(`
			SELECT id, name, address, city, state, zip_code, country, created_at, updated_at
			FROM clients
			ORDER BY name
		`)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list clients: %w", err)
		}
		defer rows.Close()

		var clients []models.Client
		for rows.Next() {
			var c models.Client
			if err := rows.Scan(&c.ID, &c.Name, &c.Address, &c.City, &c.State, &c.ZipCode, &c.Country, &c.CreatedAt, &c.UpdatedAt); err != nil {
				return nil, nil, fmt.Errorf("failed to scan client: %w", err)
			}
			clients = append(clients, c)
		}

		text := fmt.Sprintf("Found %d clients:\n", len(clients))
		for _, c := range clients {
			// Get active contracts for this client
			contractRows, err := db.Query(`
				SELECT COUNT(*) FROM contracts
				WHERE client_id = ? AND status = 'active'
			`, c.ID)
			var contractCount int
			if err == nil && contractRows.Next() {
				contractRows.Scan(&contractCount)
			}
			contractRows.Close()

			text += fmt.Sprintf("- %s (%d active contracts)\n", c.Name, contractCount)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, clients, nil
	})

	// Edit Client tool
	type editClientArgs struct {
		Name    string `json:"name" jsonschema:"Current client name"`
		NewName string `json:"new_name,omitempty" jsonschema:"New client name (optional)"`
		Address string `json:"address,omitempty" jsonschema:"New street address (optional)"`
		City    string `json:"city,omitempty" jsonschema:"New city (optional)"`
		State   string `json:"state,omitempty" jsonschema:"New state or province (optional)"`
		ZipCode string `json:"zip_code,omitempty" jsonschema:"New ZIP or postal code (optional)"`
		Country string `json:"country,omitempty" jsonschema:"New country (optional)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_client",
		Description: "Edit an existing client's information",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editClientArgs) (*mcp.CallToolResult, any, error) {
		// Get current client ID
		clientID, err := h.getClientIDByName(args.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to find client: %w", err)
		}

		// Build dynamic UPDATE query
		setParts := []string{}
		values := []interface{}{}

		if args.NewName != "" {
			setParts = append(setParts, "name = ?")
			values = append(values, args.NewName)
		}
		if args.Address != "" {
			setParts = append(setParts, "address = ?")
			values = append(values, args.Address)
		}
		if args.City != "" {
			setParts = append(setParts, "city = ?")
			values = append(values, args.City)
		}
		if args.State != "" {
			setParts = append(setParts, "state = ?")
			values = append(values, args.State)
		}
		if args.ZipCode != "" {
			setParts = append(setParts, "zip_code = ?")
			values = append(values, args.ZipCode)
		}
		if args.Country != "" {
			setParts = append(setParts, "country = ?")
			values = append(values, args.Country)
		}

		if len(setParts) == 0 {
			return nil, nil, fmt.Errorf("no fields provided to update")
		}

		// Add updated_at and client ID
		setParts = append(setParts, "updated_at = CURRENT_TIMESTAMP")
		values = append(values, clientID)

		query := fmt.Sprintf("UPDATE clients SET %s WHERE id = ?", strings.Join(setParts, ", "))

		_, err = db.Exec(query, values...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to update client: %w", err)
		}

		// Use the new name if provided, otherwise use the original name
		displayName := args.Name
		if args.NewName != "" {
			displayName = args.NewName
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully updated client: %s", displayName)},
			},
		}, nil, nil
	})

	// Add Contract tool
	type addContractArgs struct {
		ClientName     string   `json:"client_name" jsonschema:"Client name"`
		ContractNumber string   `json:"contract_number" jsonschema:"Contract number (unique identifier)"`
		Name           string   `json:"name" jsonschema:"Contract name/description"`
		HourlyRate     float64  `json:"hourly_rate" jsonschema:"Hourly rate for this contract"`
		Currency       string   `json:"currency,omitempty" jsonschema:"Currency code (e.g. USD, EUR)"`
		ContractType   string   `json:"contract_type,omitempty" jsonschema:"Contract type (hourly, fixed, retainer)"`
		StartDate      string   `json:"start_date" jsonschema:"Contract start date (YYYY-MM-DD)"`
		EndDate        string   `json:"end_date,omitempty" jsonschema:"Contract end date (YYYY-MM-DD, optional)"`
		PaymentTerms   string   `json:"payment_terms,omitempty" jsonschema:"Payment terms (e.g. 'Net 30')"`
		Notes          string   `json:"notes,omitempty" jsonschema:"Additional notes"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_contract",
		Description: "Add a new contract for a client with specific rates and terms",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addContractArgs) (*mcp.CallToolResult, any, error) {
		// Get client ID
		clientID, err := h.getClientIDByName(args.ClientName)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to find client: %w", err)
		}

		// Set defaults
		if args.Currency == "" {
			args.Currency = "USD"
		}
		if args.ContractType == "" {
			args.ContractType = "hourly"
		}

		// Parse dates
		startDate, err := time.Parse("2006-01-02", args.StartDate)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start date format: %w", err)
		}

		var endDate *time.Time
		if args.EndDate != "" {
			ed, err := time.Parse("2006-01-02", args.EndDate)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid end date format: %w", err)
			}
			endDate = &ed
		}

		// Insert contract
		var contractID int64
		err = db.QueryRow(`
			INSERT INTO contracts (client_id, contract_number, name, hourly_rate, currency, contract_type, start_date, end_date, payment_terms, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id
		`, clientID, args.ContractNumber, args.Name, args.HourlyRate, args.Currency, args.ContractType, startDate.Format("2006-01-02"),
		   func() interface{} { if endDate != nil { return endDate.Format("2006-01-02") }; return nil }(), args.PaymentTerms, args.Notes).Scan(&contractID)

		if err != nil {
			return nil, nil, fmt.Errorf("failed to add contract: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully added contract %s for %s (ID: %d)", args.ContractNumber, args.ClientName, contractID)},
			},
		}, nil, nil
	})

	// List Contracts tool
	type listContractsArgs struct {
		ClientName string `json:"client_name,omitempty" jsonschema:"Filter by client name (optional)"`
		Status     string `json:"status,omitempty" jsonschema:"Filter by status (active, completed, on_hold, cancelled)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_contracts",
		Description: "List contracts with optional filtering by client or status",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listContractsArgs) (*mcp.CallToolResult, any, error) {
		query := `
			SELECT c.id, c.contract_number, c.name, c.hourly_rate, c.currency, c.contract_type,
			       c.start_date, c.end_date, c.status, c.payment_terms, cl.name as client_name
			FROM contracts c
			JOIN clients cl ON c.client_id = cl.id
			WHERE 1=1
		`
		queryArgs := []interface{}{}

		if args.ClientName != "" {
			query += " AND cl.name LIKE ?"
			queryArgs = append(queryArgs, "%"+args.ClientName+"%")
		}

		if args.Status != "" {
			query += " AND c.status = ?"
			queryArgs = append(queryArgs, args.Status)
		}

		query += " ORDER BY c.start_date DESC, c.contract_number"

		rows, err := db.Query(query, queryArgs...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list contracts: %w", err)
		}
		defer rows.Close()

		var contracts []models.Contract
		for rows.Next() {
			var c models.Contract
			var clientName string
			var endDate *string

			err := rows.Scan(&c.ID, &c.ContractNumber, &c.Name, &c.HourlyRate, &c.Currency, &c.ContractType,
				&c.StartDate, &endDate, &c.Status, &c.PaymentTerms, &clientName)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to scan contract: %w", err)
			}

			if endDate != nil {
				ed, _ := time.Parse("2006-01-02", *endDate)
				c.EndDate = &ed
			}

			c.Client = &models.Client{Name: clientName}
			contracts = append(contracts, c)
		}

		text := fmt.Sprintf("Found %d contracts:\n", len(contracts))
		for _, c := range contracts {
			endDateStr := "ongoing"
			if c.EndDate != nil {
				endDateStr = c.EndDate.Format("2006-01-02")
			}
			text += fmt.Sprintf("- %s: %s (%s) - %s%.0f/%s [%s] %s to %s\n",
				c.ContractNumber, c.Client.Name, c.Name, c.Currency, c.HourlyRate, c.Currency,
				c.Status, c.StartDate.Format("2006-01-02"), endDateStr)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, contracts, nil
	})

	// Add Recipient tool
	type addRecipientArgs struct {
		ClientName    string `json:"client_name" jsonschema:"Client name"`
		RecipientName string `json:"recipient_name" jsonschema:"Recipient's name"`
		Email         string `json:"email" jsonschema:"Recipient's email"`
		Title         string `json:"title,omitempty" jsonschema:"Recipient's job title (optional)"`
		Phone         string `json:"phone,omitempty" jsonschema:"Recipient's phone number (optional)"`
		IsPrimary     bool   `json:"is_primary,omitempty" jsonschema:"Is this the primary recipient"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_recipient",
		Description: "Add a recipient for a client",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addRecipientArgs) (*mcp.CallToolResult, any, error) {
		clientID, err := h.getClientIDByName(args.ClientName)
		if err != nil {
			return nil, nil, fmt.Errorf("client not found: %w", err)
		}

		if args.IsPrimary {
			_, err = db.Exec(`
				UPDATE recipients SET is_primary = FALSE
				WHERE client_id = ?
			`, clientID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to update primary recipient: %w", err)
			}
		}

		result, err := db.Exec(`
			INSERT INTO recipients (client_id, name, email, title, phone, is_primary)
			VALUES (?, ?, ?, ?, ?, ?)
		`, clientID, args.RecipientName, args.Email, args.Title, args.Phone, args.IsPrimary)

		if err != nil {
			return nil, nil, fmt.Errorf("failed to add recipient: %w", err)
		}

		id, _ := result.LastInsertId()

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Recipient '%s' added for client '%s' (ID: %d)", args.RecipientName, args.ClientName, id),
				},
			},
		}, nil, nil
	})

	// List Recipients tool
	type listRecipientsArgs struct {
		ClientName string `json:"client_name" jsonschema:"Client name"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_recipients",
		Description: "List all recipients for a client",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listRecipientsArgs) (*mcp.CallToolResult, any, error) {
		clientID, err := h.getClientIDByName(args.ClientName)
		if err != nil {
			return nil, nil, fmt.Errorf("client not found: %w", err)
		}

		rows, err := db.Query(`
			SELECT id, name, email, title, phone, is_primary
			FROM recipients
			WHERE client_id = ?
			ORDER BY is_primary DESC, name
		`, clientID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list recipients: %w", err)
		}
		defer rows.Close()

		var recipients []struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			Title     string `json:"title"`
			Phone     string `json:"phone"`
			IsPrimary bool   `json:"is_primary"`
		}

		text := fmt.Sprintf("Recipients for %s:\n", args.ClientName)
		for rows.Next() {
			var r struct {
				ID        int    `json:"id"`
				Name      string `json:"name"`
				Email     string `json:"email"`
				Title     string `json:"title"`
				Phone     string `json:"phone"`
				IsPrimary bool   `json:"is_primary"`
			}
			err := rows.Scan(&r.ID, &r.Name, &r.Email, &r.Title, &r.Phone, &r.IsPrimary)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to scan recipient: %w", err)
			}
			recipients = append(recipients, r)

			primaryLabel := ""
			if r.IsPrimary {
				primaryLabel = " (PRIMARY)"
			}
			text += fmt.Sprintf("- ID %d: %s <%s>%s", r.ID, r.Name, r.Email, primaryLabel)
			if r.Title != "" {
				text += fmt.Sprintf(" - %s", r.Title)
			}
			if r.Phone != "" {
				text += fmt.Sprintf(" - %s", r.Phone)
			}
			text += "\n"
		}

		if len(recipients) == 0 {
			text += "No recipients found.\n"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, recipients, nil
	})

	// Remove Recipient tool
	type removeRecipientArgs struct {
		RecipientID int `json:"recipient_id" jsonschema:"Recipient ID to remove"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_recipient",
		Description: "Remove a recipient by ID",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeRecipientArgs) (*mcp.CallToolResult, any, error) {
		// First check if recipient exists and get details
		var name, email string
		var clientID int
		err := db.QueryRow(`
			SELECT name, email, client_id FROM recipients WHERE id = ?
		`, args.RecipientID).Scan(&name, &email, &clientID)

		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("recipient with ID %d not found", args.RecipientID)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to check recipient: %w", err)
		}

		// Remove the recipient
		result, err := db.Exec(`DELETE FROM recipients WHERE id = ?`, args.RecipientID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove recipient: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return nil, nil, fmt.Errorf("recipient with ID %d not found", args.RecipientID)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Recipient '%s <%s>' (ID: %d) removed successfully", name, email, args.RecipientID),
				},
			},
		}, nil, nil
	})

	// Set Payment Details tool
	type setPaymentDetailsArgs struct {
		ClientName     string `json:"client_name" jsonschema:"Client name"`
		BankName       string `json:"bank_name,omitempty" jsonschema:"Bank name"`
		AccountNumber  string `json:"account_number,omitempty" jsonschema:"Account number"`
		RoutingNumber  string `json:"routing_number,omitempty" jsonschema:"Routing number"`
		SwiftCode      string `json:"swift_code,omitempty" jsonschema:"SWIFT/BIC code"`
		PaymentTerms   string `json:"payment_terms,omitempty" jsonschema:"Payment terms (e.g. Net 30)"`
		Notes          string `json:"notes,omitempty" jsonschema:"Additional payment notes"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_payment_details",
		Description: "Set payment details for a client",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args setPaymentDetailsArgs) (*mcp.CallToolResult, any, error) {
		clientID, err := h.getClientIDByName(args.ClientName)
		if err != nil {
			return nil, nil, fmt.Errorf("client not found: %w", err)
		}

		_, err = db.Exec(`
			INSERT INTO payment_details (client_id, bank_name, account_number, routing_number, swift_code, payment_terms, notes, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(client_id) DO UPDATE SET
				bank_name = excluded.bank_name,
				account_number = excluded.account_number,
				routing_number = excluded.routing_number,
				swift_code = excluded.swift_code,
				payment_terms = excluded.payment_terms,
				notes = excluded.notes,
				updated_at = excluded.updated_at
		`, clientID, args.BankName, args.AccountNumber, args.RoutingNumber,
			args.SwiftCode, args.PaymentTerms, args.Notes, time.Now())

		if err != nil {
			return nil, nil, fmt.Errorf("failed to set payment details: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Payment details updated for client '%s'", args.ClientName),
				},
			},
		}, nil, nil
	})

	// Add Hours tool
	type addHoursArgs struct {
		ContractNumber string  `json:"contract_number" jsonschema:"Contract number to log hours against"`
		Hours          float64 `json:"hours" jsonschema:"Hours worked (can use 15-minute increments: 0.25, 0.5, 0.75, 1.0, 1.25, etc.)"`
		Date           string  `json:"date,omitempty" jsonschema:"Date (YYYY-MM-DD or natural language like 'today' 'yesterday')"`
		Description    string  `json:"description,omitempty" jsonschema:"Description of work done"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_hours",
		Description: "Add hours worked against a specific contract (supports 15-minute increments: 0.25 = 15 min, 0.5 = 30 min, 0.75 = 45 min)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addHoursArgs) (*mcp.CallToolResult, any, error) {
		// Get contract and verify it's active
		var contractID int
		var clientID int
		var clientName string
		var contractName string
		var status string
		err := db.QueryRow(`
			SELECT c.id, c.client_id, cl.name, c.name, c.status
			FROM contracts c
			JOIN clients cl ON c.client_id = cl.id
			WHERE c.contract_number = ?
		`, args.ContractNumber).Scan(&contractID, &clientID, &clientName, &contractName, &status)

		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("contract %s not found", args.ContractNumber)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to find contract: %w", err)
		}

		if status != "active" {
			return nil, nil, fmt.Errorf("contract %s is not active (status: %s)", args.ContractNumber, status)
		}

		date := time.Now()
		if args.Date != "" {
			date, err = timeparse.ParseDate(args.Date)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid date: %w", err)
			}
		}

		entryID := uuid.New().String()

		_, err = db.Exec(`
			INSERT INTO time_entries (id, client_id, contract_id, date, hours, description, contract_ref)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, entryID, clientID, contractID, date.Format("2006-01-02"), args.Hours, args.Description, args.ContractNumber)

		if err != nil {
			return nil, nil, fmt.Errorf("failed to add hours: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Added %.2f hours for %s (%s) on %s - %s (ID: %s)", args.Hours, clientName, contractName, date.Format("2006-01-02"), args.Description, entryID),
				},
			},
		}, nil, nil
	})

	// List Hours tool
	type listHoursArgs struct {
		ClientName string `json:"client_name,omitempty" jsonschema:"Client name (optional shows all if not specified)"`
		StartDate  string `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD or natural language)"`
		EndDate    string `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD or natural language)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_hours",
		Description: "List hours for a client within a date range",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listHoursArgs) (*mcp.CallToolResult, any, error) {
		query := `
			SELECT te.id, te.contract_id, te.date, te.hours, te.description, te.invoice_id, te.created_at,
			       cl.name, ct.contract_number, ct.name, ct.hourly_rate, ct.currency
			FROM time_entries te
			JOIN contracts ct ON te.contract_id = ct.id
			JOIN clients cl ON ct.client_id = cl.id
			WHERE 1=1
		`
		queryArgs := []interface{}{}

		if args.ClientName != "" {
			clientID, err := h.getClientIDByName(args.ClientName)
			if err != nil {
				return nil, nil, fmt.Errorf("client not found: %w", err)
			}
			query += " AND cl.id = ?"
			queryArgs = append(queryArgs, clientID)
		}

		if args.StartDate != "" {
			startDate, err := timeparse.ParseDate(args.StartDate)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid start date: %w", err)
			}
			query += " AND te.date >= ?"
			queryArgs = append(queryArgs, startDate.Format("2006-01-02"))
		}

		if args.EndDate != "" {
			endDate, err := timeparse.ParseDate(args.EndDate)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid end date: %w", err)
			}
			query += " AND te.date <= ?"
			queryArgs = append(queryArgs, endDate.Format("2006-01-02"))
		}

		query += " ORDER BY te.date DESC, te.created_at DESC"

		rows, err := db.Query(query, queryArgs...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list hours: %w", err)
		}
		defer rows.Close()

		type EntryWithContract struct {
			models.TimeEntry
			ClientName     string  `json:"client_name"`
			ContractNumber string  `json:"contract_number"`
			ContractName   string  `json:"contract_name"`
			HourlyRate     float64 `json:"hourly_rate"`
			Currency       string  `json:"currency"`
		}

		var entries []EntryWithContract
		var totalHours float64

		for rows.Next() {
			var e EntryWithContract
			if err := rows.Scan(&e.ID, &e.ContractID, &e.Date, &e.Hours, &e.Description, &e.InvoiceID, &e.CreatedAt,
				&e.ClientName, &e.ContractNumber, &e.ContractName, &e.HourlyRate, &e.Currency); err != nil {
				return nil, nil, fmt.Errorf("failed to scan entry: %w", err)
			}
			entries = append(entries, e)
			totalHours += e.Hours
		}

		text := fmt.Sprintf("Found %d entries (%.2f total hours):\n", len(entries), totalHours)
		for _, e := range entries {
			text += fmt.Sprintf("- ID %s: %s: %s - %.2f hours", e.ID, e.Date.Format("2006-01-02"), e.ClientName, e.Hours)
			if e.Description != "" {
				text += fmt.Sprintf(" (%s)", e.Description)
			}
			if e.ContractNumber != "" {
				text += fmt.Sprintf(" [Contract: %s]", e.ContractNumber)
			}
			text += "\n"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, entries, nil
	})

	// Create Invoice tool
	type createInvoiceArgs struct {
		ClientName string `json:"client_name" jsonschema:"Client name"`
		Period     string `json:"period" jsonschema:"Period (e.g. 'this month' 'last month' 'January 2025')"`
		DueDays    int    `json:"due_days,omitempty" jsonschema:"Days until due (default: 30)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_invoice",
		Description: "Create an invoice for a client",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createInvoiceArgs) (*mcp.CallToolResult, any, error) {
		if args.DueDays == 0 {
			args.DueDays = 30
		}

		clientID, err := h.getClientIDByName(args.ClientName)
		if err != nil {
			return nil, nil, fmt.Errorf("client not found: %w", err)
		}

		// Validate business information is configured
		var businessName string
		err = db.QueryRow("SELECT business_name FROM business_info WHERE id = 1").Scan(&businessName)
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("business information not configured. Please use 'set_business_info' to configure your business details before creating invoices")
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to check business info: %w", err)
		}

		// Validate payment details exist for client
		var paymentBankName string
		err = db.QueryRow("SELECT bank_name FROM payment_details WHERE client_id = ?", clientID).Scan(&paymentBankName)
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("payment details not configured for client '%s'. Please use 'set_payment_details' to configure payment information before creating invoices", args.ClientName)
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to check payment details: %w", err)
		}

		startDate, endDate, err := timeparse.ParsePeriod(args.Period)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid period: %w", err)
		}

		var client models.Client
		err = db.QueryRow(`
			SELECT id, name, address, city, state, zip_code, country
			FROM clients WHERE id = ?
		`, clientID).Scan(&client.ID, &client.Name, &client.Address, &client.City, &client.State, &client.ZipCode, &client.Country)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get client details: %w", err)
		}

		rows, err := db.Query(`
			SELECT te.id, te.date, te.hours, te.description, ct.hourly_rate, ct.currency
			FROM time_entries te
			JOIN contracts ct ON te.contract_id = ct.id
			WHERE ct.client_id = ? AND te.date >= ? AND te.date <= ? AND te.invoice_id IS NULL
			ORDER BY te.date
		`, clientID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get time entries: %w", err)
		}
		defer rows.Close()

		var entries []models.TimeEntry
		var totalHours float64
		var totalAmount float64
		for rows.Next() {
			var e models.TimeEntry
			var hourlyRate float64
			var currency string
			if err := rows.Scan(&e.ID, &e.Date, &e.Hours, &e.Description, &hourlyRate, &currency); err != nil {
				return nil, nil, fmt.Errorf("failed to scan entry: %w", err)
			}
			entries = append(entries, e)
			totalHours += e.Hours
			totalAmount += e.Hours * hourlyRate
		}

		if len(entries) == 0 {
			return nil, nil, fmt.Errorf("no unbilled hours found for %s in %s", args.ClientName, args.Period)
		}
		invoiceNumber := fmt.Sprintf("INV-%s-%s", time.Now().Format("200601"), uuid.New().String()[:8])
		issueDate := time.Now()
		dueDate := issueDate.AddDate(0, 0, args.DueDays)

		tx, err := db.Begin()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		result, err := tx.Exec(`
			INSERT INTO invoices (client_id, invoice_number, issue_date, due_date, total_amount, status)
			VALUES (?, ?, ?, ?, ?, 'pending')
		`, clientID, invoiceNumber, issueDate.Format("2006-01-02"), dueDate.Format("2006-01-02"), totalAmount)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create invoice: %w", err)
		}

		invoiceID, _ := result.LastInsertId()

		invoice := models.Invoice{
			ID:            int(invoiceID),
			ClientID:      clientID,
			InvoiceNumber: invoiceNumber,
			IssueDate:     issueDate,
			DueDate:       dueDate,
			TotalAmount:   totalAmount,
			Status:        "pending",
			Client:        &client,
			TimeEntries:   entries,
		}

		var paymentDetails models.PaymentDetails
		db.QueryRow(`
			SELECT bank_name, account_number, routing_number, swift_code, payment_terms, notes
			FROM payment_details WHERE client_id = ?
		`, clientID).Scan(&paymentDetails.BankName, &paymentDetails.AccountNumber,
			&paymentDetails.RoutingNumber, &paymentDetails.SwiftCode,
			&paymentDetails.PaymentTerms, &paymentDetails.Notes)

		var recipients []models.Recipient
		recipientRows, err := db.Query(`
			SELECT name, email, title, phone FROM recipients
			WHERE client_id = ? ORDER BY is_primary DESC
		`, clientID)
		if err == nil {
			defer recipientRows.Close()
			for recipientRows.Next() {
				var r models.Recipient
				recipientRows.Scan(&r.Name, &r.Email, &r.Title, &r.Phone)
				recipients = append(recipients, r)
			}
		}

		var business models.BusinessInfo
		db.QueryRow(`
			SELECT id, business_name, contact_name, email, phone, address, city, state, zip_code, country, tax_id, website, logo_path, invoice_prefix, updated_at
			FROM business_info WHERE id = 1
		`).Scan(&business.ID, &business.BusinessName, &business.ContactName, &business.Email,
			&business.Phone, &business.Address, &business.City, &business.State,
			&business.ZipCode, &business.Country, &business.TaxID, &business.Website,
			&business.LogoPath, &business.InvoicePrefix, &business.UpdatedAt)

		homeDir, _ := os.UserHomeDir()
		downloadsPath := filepath.Join(homeDir, "Downloads")
		pdfPath := filepath.Join(downloadsPath, fmt.Sprintf("invoice_%s.pdf", issueDate.Format("2006-01-02")))

		// Link time entries to the invoice
		for _, entry := range entries {
			_, err = tx.Exec(`UPDATE time_entries SET invoice_id = ? WHERE id = ?`, invoiceID, entry.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to link time entry to invoice: %w", err)
			}
		}

		// Update invoice entries with contract info for PDF generation
		for i := range entries {
			var contract models.Contract
			err = tx.QueryRow(`
				SELECT c.id, c.contract_number, c.name, c.hourly_rate, c.currency, c.payment_terms
				FROM contracts c
				JOIN time_entries te ON te.contract_id = c.id
				WHERE te.id = ?
			`, entries[i].ID).Scan(&contract.ID, &contract.ContractNumber, &contract.Name,
				&contract.HourlyRate, &contract.Currency, &contract.PaymentTerms)
			if err == nil {
				entries[i].Contract = &contract
			}
		}
		invoice.TimeEntries = entries

		generator := pdf.NewInvoiceGenerator()
		if err := generator.Generate(invoice, paymentDetails, recipients, business, pdfPath); err != nil {
			return nil, nil, fmt.Errorf("failed to generate PDF: %w", err)
		}

		tx.Exec(`UPDATE invoices SET pdf_path = ? WHERE id = ?`, pdfPath, invoiceID)

		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Invoice %s created successfully\nTotal: $%.2f (%.2f hours)\nPDF saved to: %s",
						invoiceNumber, totalAmount, totalHours, pdfPath),
				},
			},
		}, map[string]interface{}{
			"invoice_number": invoiceNumber,
			"total_amount":   totalAmount,
			"total_hours":    totalHours,
			"pdf_path":       pdfPath,
		}, nil
	})

	// Delete Time Entry tool
	type deleteTimeEntryArgs struct {
		EntryID string `json:"entry_id" jsonschema:"Time entry UUID to delete"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_time_entry",
		Description: "Delete a specific time entry by ID",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteTimeEntryArgs) (*mcp.CallToolResult, any, error) {
		var clientName string
		var date, hours, description string
		err := db.QueryRow(`
			SELECT c.name, te.date, te.hours, te.description
			FROM time_entries te
			JOIN clients c ON te.client_id = c.id
			WHERE te.id = ?
		`, args.EntryID).Scan(&clientName, &date, &hours, &description)

		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("time entry with ID %s not found", args.EntryID)
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to find time entry: %w", err)
		}

		result, err := db.Exec("DELETE FROM time_entries WHERE id = ?", args.EntryID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to delete time entry: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return nil, nil, fmt.Errorf("time entry with ID %s not found", args.EntryID)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Deleted time entry ID %s: %s - %s hours on %s (%s)",
						args.EntryID, clientName, hours, date, description),
				},
			},
		}, nil, nil
	})

	// Bulk Delete Time Entries tool
	type bulkDeleteTimeEntriesArgs struct {
		EntryIDs []string `json:"entry_ids" jsonschema:"List of time entry UUIDs to delete"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "bulk_delete_time_entries",
		Description: "Delete multiple time entries by their IDs",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args bulkDeleteTimeEntriesArgs) (*mcp.CallToolResult, any, error) {
		if len(args.EntryIDs) == 0 {
			return nil, nil, fmt.Errorf("no entry IDs provided")
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		var deletedEntries []string
		var deletedCount int

		for _, entryID := range args.EntryIDs {
			var clientName string
			var date, hours, description string
			err := tx.QueryRow(`
				SELECT c.name, te.date, te.hours, te.description
				FROM time_entries te
				JOIN clients c ON te.client_id = c.id
				WHERE te.id = ?
			`, entryID).Scan(&clientName, &date, &hours, &description)

			if err == sql.ErrNoRows {
				continue
			} else if err != nil {
				return nil, nil, fmt.Errorf("failed to find time entry %s: %w", entryID, err)
			}

			result, err := tx.Exec("DELETE FROM time_entries WHERE id = ?", entryID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to delete time entry %s: %w", entryID, err)
			}

			rowsAffected, _ := result.RowsAffected()
			if rowsAffected > 0 {
				deletedEntries = append(deletedEntries,
					fmt.Sprintf("ID %s: %s - %s hours on %s (%s)",
						entryID, clientName, hours, date, description))
				deletedCount++
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
		}

		text := fmt.Sprintf("Deleted %d time entries:\n", deletedCount)
		for _, entry := range deletedEntries {
			text += fmt.Sprintf("- %s\n", entry)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"deleted_count": deletedCount,
			"deleted_entries": deletedEntries,
		}, nil
	})

	// Bulk Add Hours tool
	type bulkAddHoursEntry struct {
		ClientName  string  `json:"client_name" jsonschema:"Client name"`
		Hours       float64 `json:"hours" jsonschema:"Hours worked (can use 15-minute increments: 0.25, 0.5, 0.75, 1.0, 1.25, etc.)"`
		Date        string  `json:"date" jsonschema:"Date (YYYY-MM-DD or natural language like 'today' 'yesterday')"`
		Description string  `json:"description,omitempty" jsonschema:"Description of work done"`
		ContractRef string  `json:"contract_ref,omitempty" jsonschema:"Contract reference number (optional)"`
	}

	type bulkAddHoursArgs struct {
		Entries []bulkAddHoursEntry `json:"entries" jsonschema:"List of time entries to add"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "bulk_add_hours",
		Description: "Add multiple time entries at once (supports 15-minute increments: 0.25 = 15 min, 0.5 = 30 min, 0.75 = 45 min)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args bulkAddHoursArgs) (*mcp.CallToolResult, any, error) {
		if len(args.Entries) == 0 {
			return nil, nil, fmt.Errorf("no entries provided")
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		var addedEntries []string
		var addedCount int
		var totalHours float64

		for _, entry := range args.Entries {
			clientID, err := h.getClientIDByName(entry.ClientName)
			if err != nil {
				return nil, nil, fmt.Errorf("client '%s' not found: %w", entry.ClientName, err)
			}

			// Look up contract ID by contract number
			var contractID int
			err = tx.QueryRow("SELECT id FROM contracts WHERE contract_number = ?", entry.ContractRef).Scan(&contractID)
			if err != nil {
				return nil, nil, fmt.Errorf("contract '%s' not found: %w", entry.ContractRef, err)
			}

			date := time.Now()
			if entry.Date != "" {
				date, err = timeparse.ParseDate(entry.Date)
				if err != nil {
					return nil, nil, fmt.Errorf("invalid date '%s': %w", entry.Date, err)
				}
			}

			entryID := uuid.New().String()

			_, err = tx.Exec(`
				INSERT INTO time_entries (id, client_id, contract_id, date, hours, description, contract_ref)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, entryID, clientID, contractID, date.Format("2006-01-02"), entry.Hours, entry.Description, entry.ContractRef)

			if err != nil {
				return nil, nil, fmt.Errorf("failed to add entry for %s: %w", entry.ClientName, err)
			}

			addedEntries = append(addedEntries,
				fmt.Sprintf("ID %s: %s - %.2f hours on %s (%s)",
					entryID, entry.ClientName, entry.Hours, date.Format("2006-01-02"), entry.Description))
			addedCount++
			totalHours += entry.Hours
		}

		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
		}

		text := fmt.Sprintf("Added %d time entries (%.2f total hours):\n", addedCount, totalHours)
		for _, entry := range addedEntries {
			text += fmt.Sprintf("- %s\n", entry)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"added_count": addedCount,
			"total_hours": totalHours,
			"added_entries": addedEntries,
		}, nil
	})

	// Helper: Get Time Entry Details tool
	type getTimeEntryDetailsArgs struct {
		EntryID string `json:"entry_id" jsonschema:"Time entry UUID to get details for"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_time_entry_details",
		Description: "Get detailed information about a specific time entry",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getTimeEntryDetailsArgs) (*mcp.CallToolResult, any, error) {
		var entry models.TimeEntry
		var clientName string

		err := db.QueryRow(`
			SELECT te.id, te.contract_id, te.date, te.hours, te.description, te.invoice_id, te.created_at, cl.name
			FROM time_entries te
			JOIN contracts ct ON te.contract_id = ct.id
			JOIN clients cl ON ct.client_id = cl.id
			WHERE te.id = ?
		`, args.EntryID).Scan(&entry.ID, &entry.ContractID, &entry.Date, &entry.Hours,
			&entry.Description, &entry.InvoiceID, &entry.CreatedAt, &clientName)

		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("time entry with ID %s not found", args.EntryID)
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to get time entry details: %w", err)
		}

		invoiceStatus := "Not invoiced"
		if entry.InvoiceID != nil {
			var invoiceNumber string
			db.QueryRow("SELECT invoice_number FROM invoices WHERE id = ?", *entry.InvoiceID).Scan(&invoiceNumber)
			invoiceStatus = fmt.Sprintf("Invoiced (%s)", invoiceNumber)
		}

		text := fmt.Sprintf("Time Entry Details (ID: %s):\n", entry.ID)
		text += fmt.Sprintf("Client: %s\n", clientName)
		text += fmt.Sprintf("Date: %s\n", entry.Date.Format("2006-01-02"))
		text += fmt.Sprintf("Hours: %.2f\n", entry.Hours)
		text += fmt.Sprintf("Description: %s\n", entry.Description)
		// Contract info now handled differently - could add contract details here if needed
		text += fmt.Sprintf("Invoice Status: %s\n", invoiceStatus)
		text += fmt.Sprintf("Created: %s\n", entry.CreatedAt.Format("2006-01-02 15:04:05"))

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"entry": entry,
			"client_name": clientName,
			"invoice_status": invoiceStatus,
		}, nil
	})

	// Helper: Update Time Entry tool
	type updateTimeEntryArgs struct {
		EntryID     string  `json:"entry_id" jsonschema:"Time entry UUID to update"`
		Hours       *float64 `json:"hours,omitempty" jsonschema:"New hours value in 15-minute increments: 0.25, 0.5, 0.75, 1.0, etc. (optional)"`
		Date        string  `json:"date,omitempty" jsonschema:"New date (optional, YYYY-MM-DD or natural language)"`
		Description *string `json:"description,omitempty" jsonschema:"New description (optional)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_time_entry",
		Description: "Update an existing time entry (hours support 15-minute increments: 0.25 = 15 min, 0.5 = 30 min, 0.75 = 45 min)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateTimeEntryArgs) (*mcp.CallToolResult, any, error) {
		var entry models.TimeEntry
		var clientName string

		err := db.QueryRow(`
			SELECT te.id, te.contract_id, te.date, te.hours, te.description, te.invoice_id, cl.name
			FROM time_entries te
			JOIN contracts ct ON te.contract_id = ct.id
			JOIN clients cl ON ct.client_id = cl.id
			WHERE te.id = ?
		`, args.EntryID).Scan(&entry.ID, &entry.ContractID, &entry.Date, &entry.Hours,
			&entry.Description, &entry.InvoiceID, &clientName)

		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("time entry with ID %s not found", args.EntryID)
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to find time entry: %w", err)
		}

		if entry.InvoiceID != nil {
			return nil, nil, fmt.Errorf("cannot update time entry that has already been invoiced")
		}

		updates := []string{}
		updateArgs := []interface{}{}

		if args.Hours != nil {
			updates = append(updates, "hours = ?")
			updateArgs = append(updateArgs, *args.Hours)
		}

		if args.Date != "" {
			date, err := timeparse.ParseDate(args.Date)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid date: %w", err)
			}
			updates = append(updates, "date = ?")
			updateArgs = append(updateArgs, date.Format("2006-01-02"))
		}

		if args.Description != nil {
			updates = append(updates, "description = ?")
			updateArgs = append(updateArgs, *args.Description)
		}

		if len(updates) == 0 {
			return nil, nil, fmt.Errorf("no updates provided")
		}

		updateArgs = append(updateArgs, args.EntryID)
		query := fmt.Sprintf("UPDATE time_entries SET %s WHERE id = ?",
			strings.Join(updates, ", "))

		_, err = db.Exec(query, updateArgs...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to update time entry: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Updated time entry ID %s for %s", args.EntryID, clientName),
				},
			},
		}, nil, nil
	})

	// Helper: Search Time Entries tool
	type searchTimeEntriesArgs struct {
		ClientName  string `json:"client_name,omitempty" jsonschema:"Client name to filter by (optional)"`
		Description string `json:"description,omitempty" jsonschema:"Search description text (optional)"`
		ContractRef string `json:"contract_ref,omitempty" jsonschema:"Contract reference to filter by (optional)"`
		MinHours    *float64 `json:"min_hours,omitempty" jsonschema:"Minimum hours (optional)"`
		MaxHours    *float64 `json:"max_hours,omitempty" jsonschema:"Maximum hours (optional)"`
		StartDate   string `json:"start_date,omitempty" jsonschema:"Start date (optional)"`
		EndDate     string `json:"end_date,omitempty" jsonschema:"End date (optional)"`
		Invoiced    *bool  `json:"invoiced,omitempty" jsonschema:"Filter by invoice status: true=invoiced, false=not invoiced, null=all (optional)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_time_entries",
		Description: "Search time entries with various filters",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchTimeEntriesArgs) (*mcp.CallToolResult, any, error) {
		query := `
			SELECT te.id, te.contract_id, te.date, te.hours, te.description, te.invoice_id, te.created_at,
			       cl.name, ct.contract_number, ct.name, ct.hourly_rate, ct.currency
			FROM time_entries te
			JOIN contracts ct ON te.contract_id = ct.id
			JOIN clients cl ON ct.client_id = cl.id
			WHERE 1=1
		`
		queryArgs := []interface{}{}

		if args.ClientName != "" {
			clientID, err := h.getClientIDByName(args.ClientName)
			if err != nil {
				return nil, nil, fmt.Errorf("client not found: %w", err)
			}
			query += " AND cl.id = ?"
			queryArgs = append(queryArgs, clientID)
		}

		if args.Description != "" {
			query += " AND te.description LIKE ?"
			queryArgs = append(queryArgs, "%"+args.Description+"%")
		}

		if args.ContractRef != "" {
			query += " AND ct.contract_number LIKE ?"
			queryArgs = append(queryArgs, "%"+args.ContractRef+"%")
		}

		if args.MinHours != nil {
			query += " AND te.hours >= ?"
			queryArgs = append(queryArgs, *args.MinHours)
		}

		if args.MaxHours != nil {
			query += " AND te.hours <= ?"
			queryArgs = append(queryArgs, *args.MaxHours)
		}

		if args.StartDate != "" {
			startDate, err := timeparse.ParseDate(args.StartDate)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid start date: %w", err)
			}
			query += " AND te.date >= ?"
			queryArgs = append(queryArgs, startDate.Format("2006-01-02"))
		}

		if args.EndDate != "" {
			endDate, err := timeparse.ParseDate(args.EndDate)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid end date: %w", err)
			}
			query += " AND te.date <= ?"
			queryArgs = append(queryArgs, endDate.Format("2006-01-02"))
		}

		if args.Invoiced != nil {
			if *args.Invoiced {
				query += " AND te.invoice_id IS NOT NULL"
			} else {
				query += " AND te.invoice_id IS NULL"
			}
		}

		query += " ORDER BY te.date DESC, te.created_at DESC"

		rows, err := db.Query(query, queryArgs...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to search time entries: %w", err)
		}
		defer rows.Close()

		type EntryWithContract struct {
			models.TimeEntry
			ClientName     string  `json:"client_name"`
			ContractNumber string  `json:"contract_number"`
			ContractName   string  `json:"contract_name"`
			HourlyRate     float64 `json:"hourly_rate"`
			Currency       string  `json:"currency"`
		}

		var entries []EntryWithContract
		var totalHours float64

		for rows.Next() {
			var e EntryWithContract
			if err := rows.Scan(&e.ID, &e.ContractID, &e.Date, &e.Hours, &e.Description, &e.InvoiceID, &e.CreatedAt,
				&e.ClientName, &e.ContractNumber, &e.ContractName, &e.HourlyRate, &e.Currency); err != nil {
				return nil, nil, fmt.Errorf("failed to scan entry: %w", err)
			}
			entries = append(entries, e)
			totalHours += e.Hours
		}

		text := fmt.Sprintf("Found %d entries (%.2f total hours):\n", len(entries), totalHours)
		for _, e := range entries {
			invoiceStatus := "Not invoiced"
			if e.InvoiceID != nil {
				invoiceStatus = fmt.Sprintf("Invoiced (ID: %d)", *e.InvoiceID)
			}
			text += fmt.Sprintf("- ID %s: %s: %s - %.2f hours (%s)",
				e.ID, e.Date.Format("2006-01-02"), e.ClientName, e.Hours, invoiceStatus)
			if e.Description != "" {
				text += fmt.Sprintf(" - %s", e.Description)
			}
			if e.ContractNumber != "" {
				text += fmt.Sprintf(" [Contract: %s]", e.ContractNumber)
			}
			text += "\n"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"entries": entries,
			"total_hours": totalHours,
			"count": len(entries),
		}, nil
	})

	// Mark Time Entries as Invoiced tool
	type markTimeEntriesInvoicedArgs struct {
		InvoiceNumber string   `json:"invoice_number" jsonschema:"Invoice number to link entries to"`
		EntryIDs      []string `json:"entry_ids" jsonschema:"List of time entry UUIDs to mark as invoiced"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_time_entries_invoiced",
		Description: "Mark specific time entries as invoiced by linking them to an invoice",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args markTimeEntriesInvoicedArgs) (*mcp.CallToolResult, any, error) {
		if len(args.EntryIDs) == 0 {
			return nil, nil, fmt.Errorf("no entry IDs provided")
		}

		var invoiceID int
		err := db.QueryRow("SELECT id FROM invoices WHERE invoice_number = ?", args.InvoiceNumber).Scan(&invoiceID)
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("invoice %s not found", args.InvoiceNumber)
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to find invoice: %w", err)
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		var markedEntries []string
		var markedCount int

		for _, entryID := range args.EntryIDs {
			var clientName string
			var date string
			var hours float64
			var description string
			var currentInvoiceID *int

			err := tx.QueryRow(`
				SELECT c.name, te.date, te.hours, te.description, te.invoice_id
				FROM time_entries te
				JOIN clients c ON te.client_id = c.id
				WHERE te.id = ?
			`, entryID).Scan(&clientName, &date, &hours, &description, &currentInvoiceID)

			if err == sql.ErrNoRows {
				continue
			} else if err != nil {
				return nil, nil, fmt.Errorf("failed to find time entry %s: %w", entryID, err)
			}

			if currentInvoiceID != nil {
				var currentInvoiceNumber string
				tx.QueryRow("SELECT invoice_number FROM invoices WHERE id = ?", *currentInvoiceID).Scan(&currentInvoiceNumber)
				return nil, nil, fmt.Errorf("time entry %s is already invoiced (%s)", entryID, currentInvoiceNumber)
			}

			result, err := tx.Exec("UPDATE time_entries SET invoice_id = ? WHERE id = ?", invoiceID, entryID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to mark time entry %s as invoiced: %w", entryID, err)
			}

			rowsAffected, _ := result.RowsAffected()
			if rowsAffected > 0 {
				markedEntries = append(markedEntries,
					fmt.Sprintf("ID %s: %s - %.2f hours on %s (%s)",
						entryID, clientName, hours, date, description))
				markedCount++
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
		}

		text := fmt.Sprintf("Marked %d time entries as invoiced (%s):\n", markedCount, args.InvoiceNumber)
		for _, entry := range markedEntries {
			text += fmt.Sprintf("- %s\n", entry)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"marked_count": markedCount,
			"invoice_number": args.InvoiceNumber,
			"marked_entries": markedEntries,
		}, nil
	})

	// Unmark Time Entries from Invoice tool
	type unmarkTimeEntriesArgs struct {
		EntryIDs []string `json:"entry_ids" jsonschema:"List of time entry UUIDs to unmark from invoices"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "unmark_time_entries_from_invoice",
		Description: "Remove invoice association from time entries, making them available for billing again",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args unmarkTimeEntriesArgs) (*mcp.CallToolResult, any, error) {
		if len(args.EntryIDs) == 0 {
			return nil, nil, fmt.Errorf("no entry IDs provided")
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		var unmarkedEntries []string
		var unmarkedCount int

		for _, entryID := range args.EntryIDs {
			var clientName string
			var date string
			var hours float64
			var description string
			var invoiceNumber *string

			err := tx.QueryRow(`
				SELECT c.name, te.date, te.hours, te.description, i.invoice_number
				FROM time_entries te
				JOIN clients c ON te.client_id = c.id
				LEFT JOIN invoices i ON te.invoice_id = i.id
				WHERE te.id = ?
			`, entryID).Scan(&clientName, &date, &hours, &description, &invoiceNumber)

			if err == sql.ErrNoRows {
				continue
			} else if err != nil {
				return nil, nil, fmt.Errorf("failed to find time entry %s: %w", entryID, err)
			}

			result, err := tx.Exec("UPDATE time_entries SET invoice_id = NULL WHERE id = ?", entryID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to unmark time entry %s: %w", entryID, err)
			}

			rowsAffected, _ := result.RowsAffected()
			if rowsAffected > 0 {
				invoiceInfo := "No invoice"
				if invoiceNumber != nil {
					invoiceInfo = fmt.Sprintf("was %s", *invoiceNumber)
				}
				unmarkedEntries = append(unmarkedEntries,
					fmt.Sprintf("ID %s: %s - %.2f hours on %s (%s) [%s]",
						entryID, clientName, hours, date, description, invoiceInfo))
				unmarkedCount++
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
		}

		text := fmt.Sprintf("Unmarked %d time entries from invoices:\n", unmarkedCount)
		for _, entry := range unmarkedEntries {
			text += fmt.Sprintf("- %s\n", entry)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"unmarked_count": unmarkedCount,
			"unmarked_entries": unmarkedEntries,
		}, nil
	})

	// List Invoice Details tool
	type listInvoiceDetailsArgs struct {
		InvoiceNumber string `json:"invoice_number" jsonschema:"Invoice number to get details for"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_invoice_details",
		Description: "Get detailed information about an invoice including all associated time entries",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listInvoiceDetailsArgs) (*mcp.CallToolResult, any, error) {
		var invoice models.Invoice
		var clientName string

		err := db.QueryRow(`
			SELECT i.id, i.client_id, i.invoice_number, i.issue_date, i.due_date,
				   i.total_amount, i.status, i.pdf_path, i.created_at, c.name
			FROM invoices i
			JOIN clients c ON i.client_id = c.id
			WHERE i.invoice_number = ?
		`, args.InvoiceNumber).Scan(&invoice.ID, &invoice.ClientID, &invoice.InvoiceNumber,
			&invoice.IssueDate, &invoice.DueDate, &invoice.TotalAmount,
			&invoice.Status, &invoice.PDFPath, &invoice.CreatedAt, &clientName)

		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("invoice %s not found", args.InvoiceNumber)
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to get invoice details: %w", err)
		}

		rows, err := db.Query(`
			SELECT te.id, te.date, te.hours, te.description, te.contract_ref
			FROM time_entries te
			WHERE te.invoice_id = ?
			ORDER BY te.date
		`, invoice.ID)

		var entries []models.TimeEntry
		var totalHours float64
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var e models.TimeEntry
				if err := rows.Scan(&e.ID, &e.Date, &e.Hours, &e.Description); err == nil {
					// Note: ContractRef field no longer exists in TimeEntry model
					entries = append(entries, e)
					totalHours += e.Hours
				}
			}
		}

		text := fmt.Sprintf("Invoice Details: %s\n", invoice.InvoiceNumber)
		text += fmt.Sprintf("Client: %s\n", clientName)
		text += fmt.Sprintf("Issue Date: %s\n", invoice.IssueDate.Format("2006-01-02"))
		text += fmt.Sprintf("Due Date: %s\n", invoice.DueDate.Format("2006-01-02"))
		text += fmt.Sprintf("Status: %s\n", invoice.Status)
		text += fmt.Sprintf("Total Amount: $%.2f\n", invoice.TotalAmount)
		text += fmt.Sprintf("Total Hours: %.2f\n", totalHours)
		if invoice.PDFPath != "" {
			text += fmt.Sprintf("PDF Path: %s\n", invoice.PDFPath)
		}
		text += fmt.Sprintf("\nTime Entries (%d):\n", len(entries))
		for _, e := range entries {
			text += fmt.Sprintf("- ID %s: %s - %.2f hours (%s)\n",
				e.ID, e.Date.Format("2006-01-02"), e.Hours, e.Description)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"invoice": invoice,
			"client_name": clientName,
			"time_entries": entries,
			"total_hours": totalHours,
		}, nil
	})

	// Set Business Info tool
	type setBusinessInfoArgs struct {
		BusinessName  string `json:"business_name" jsonschema:"Business name"`
		ContactName   string `json:"contact_name" jsonschema:"Contact person name"`
		Email         string `json:"email" jsonschema:"Business email address"`
		Phone         string `json:"phone,omitempty" jsonschema:"Phone number (optional)"`
		Address       string `json:"address,omitempty" jsonschema:"Street address (optional)"`
		City          string `json:"city,omitempty" jsonschema:"City (optional)"`
		State         string `json:"state,omitempty" jsonschema:"State/Province (optional)"`
		ZipCode       string `json:"zip_code,omitempty" jsonschema:"ZIP/Postal code (optional)"`
		Country       string `json:"country,omitempty" jsonschema:"Country (optional)"`
		TaxID         string `json:"tax_id,omitempty" jsonschema:"Tax ID/EIN (optional)"`
		Website       string `json:"website,omitempty" jsonschema:"Website URL (optional)"`
		LogoPath      string `json:"logo_path,omitempty" jsonschema:"Path to logo file (optional)"`
		InvoicePrefix string `json:"invoice_prefix,omitempty" jsonschema:"Invoice number prefix (optional, defaults to 'INV')"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_business_info",
		Description: "Set or update your business information for invoices",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args setBusinessInfoArgs) (*mcp.CallToolResult, any, error) {
		if args.InvoicePrefix == "" {
			args.InvoicePrefix = "INV"
		}

		_, err := db.Exec(`
			INSERT INTO business_info (id, business_name, contact_name, email, phone, address, city, state, zip_code, country, tax_id, website, logo_path, invoice_prefix, updated_at)
			VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				business_name = excluded.business_name,
				contact_name = excluded.contact_name,
				email = excluded.email,
				phone = excluded.phone,
				address = excluded.address,
				city = excluded.city,
				state = excluded.state,
				zip_code = excluded.zip_code,
				country = excluded.country,
				tax_id = excluded.tax_id,
				website = excluded.website,
				logo_path = excluded.logo_path,
				invoice_prefix = excluded.invoice_prefix,
				updated_at = excluded.updated_at
		`, args.BusinessName, args.ContactName, args.Email, args.Phone, args.Address,
			args.City, args.State, args.ZipCode, args.Country, args.TaxID,
			args.Website, args.LogoPath, args.InvoicePrefix, time.Now())

		if err != nil {
			return nil, nil, fmt.Errorf("failed to set business info: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Business information updated successfully for '%s'", args.BusinessName),
				},
			},
		}, nil, nil
	})

	// Get Business Info tool
	type getBusinessInfoArgs struct{}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_business_info",
		Description: "Get current business information settings",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getBusinessInfoArgs) (*mcp.CallToolResult, any, error) {
		var business models.BusinessInfo
		err := db.QueryRow(`
			SELECT id, business_name, contact_name, email, phone, address, city, state, zip_code, country, tax_id, website, logo_path, invoice_prefix, updated_at
			FROM business_info WHERE id = 1
		`).Scan(&business.ID, &business.BusinessName, &business.ContactName, &business.Email,
			&business.Phone, &business.Address, &business.City, &business.State,
			&business.ZipCode, &business.Country, &business.TaxID, &business.Website,
			&business.LogoPath, &business.InvoicePrefix, &business.UpdatedAt)

		if err == sql.ErrNoRows {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: "No business information configured. Use 'set_business_info' to configure your business details.",
					},
				},
			}, nil, nil
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to get business info: %w", err)
		}

		text := fmt.Sprintf("Business Information:\n")
		text += fmt.Sprintf("Business Name: %s\n", business.BusinessName)
		text += fmt.Sprintf("Contact: %s\n", business.ContactName)
		text += fmt.Sprintf("Email: %s\n", business.Email)
		if business.Phone != "" {
			text += fmt.Sprintf("Phone: %s\n", business.Phone)
		}
		if business.Address != "" {
			text += fmt.Sprintf("Address: %s\n", business.Address)
		}
		if business.City != "" {
			text += fmt.Sprintf("City: %s\n", business.City)
		}
		if business.State != "" {
			text += fmt.Sprintf("State: %s\n", business.State)
		}
		if business.ZipCode != "" {
			text += fmt.Sprintf("ZIP: %s\n", business.ZipCode)
		}
		if business.Country != "" {
			text += fmt.Sprintf("Country: %s\n", business.Country)
		}
		if business.TaxID != "" {
			text += fmt.Sprintf("Tax ID: %s\n", business.TaxID)
		}
		if business.Website != "" {
			text += fmt.Sprintf("Website: %s\n", business.Website)
		}
		if business.LogoPath != "" {
			text += fmt.Sprintf("Logo: %s\n", business.LogoPath)
		}
		text += fmt.Sprintf("Invoice Prefix: %s\n", business.InvoicePrefix)
		text += fmt.Sprintf("Last Updated: %s\n", business.UpdatedAt.Format("2006-01-02 15:04:05"))

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, business, nil
	})

	// Update Invoice Status tool
	type updateInvoiceStatusArgs struct {
		InvoiceNumber string `json:"invoice_number" jsonschema:"Invoice number to update"`
		Status        string `json:"status" jsonschema:"New status (draft, sent, paid, overdue, cancelled)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_invoice_status",
		Description: "Update the status of an invoice",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateInvoiceStatusArgs) (*mcp.CallToolResult, any, error) {
		validStatuses := map[string]bool{
			"draft":    true,
			"sent":     true,
			"paid":     true,
			"overdue":  true,
			"cancelled": true,
		}

		if !validStatuses[args.Status] {
			return nil, nil, fmt.Errorf("invalid status '%s'. Valid statuses are: draft, sent, paid, overdue, cancelled", args.Status)
		}

		result, err := db.Exec(`
			UPDATE invoices SET status = ? WHERE invoice_number = ?
		`, args.Status, args.InvoiceNumber)

		if err != nil {
			return nil, nil, fmt.Errorf("failed to update invoice status: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return nil, nil, fmt.Errorf("invoice %s not found", args.InvoiceNumber)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Invoice %s status updated to '%s'", args.InvoiceNumber, args.Status),
				},
			},
		}, nil, nil
	})

	// List Invoices tool
	type listInvoicesArgs struct {
		ClientName string `json:"client_name,omitempty" jsonschema:"Filter by client name (optional)"`
		Status     string `json:"status,omitempty" jsonschema:"Filter by status (optional)"`
		StartDate  string `json:"start_date,omitempty" jsonschema:"Filter by issue date start (optional)"`
		EndDate    string `json:"end_date,omitempty" jsonschema:"Filter by issue date end (optional)"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_invoices",
		Description: "List invoices with optional filters",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listInvoicesArgs) (*mcp.CallToolResult, any, error) {
		query := `
			SELECT i.id, i.invoice_number, i.issue_date, i.due_date, i.total_amount, i.status, c.name
			FROM invoices i
			JOIN clients c ON i.client_id = c.id
			WHERE 1=1
		`
		queryArgs := []interface{}{}

		if args.ClientName != "" {
			clientID, err := h.getClientIDByName(args.ClientName)
			if err != nil {
				return nil, nil, fmt.Errorf("client not found: %w", err)
			}
			query += " AND i.client_id = ?"
			queryArgs = append(queryArgs, clientID)
		}

		if args.Status != "" {
			query += " AND i.status = ?"
			queryArgs = append(queryArgs, args.Status)
		}

		if args.StartDate != "" {
			startDate, err := timeparse.ParseDate(args.StartDate)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid start date: %w", err)
			}
			query += " AND i.issue_date >= ?"
			queryArgs = append(queryArgs, startDate.Format("2006-01-02"))
		}

		if args.EndDate != "" {
			endDate, err := timeparse.ParseDate(args.EndDate)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid end date: %w", err)
			}
			query += " AND i.issue_date <= ?"
			queryArgs = append(queryArgs, endDate.Format("2006-01-02"))
		}

		query += " ORDER BY i.issue_date DESC"

		rows, err := db.Query(query, queryArgs...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list invoices: %w", err)
		}
		defer rows.Close()

		type InvoiceWithClient struct {
			ID            int       `json:"id"`
			InvoiceNumber string    `json:"invoice_number"`
			IssueDate     time.Time `json:"issue_date"`
			DueDate       time.Time `json:"due_date"`
			TotalAmount   float64   `json:"total_amount"`
			Status        string    `json:"status"`
			ClientName    string    `json:"client_name"`
		}

		var invoices []InvoiceWithClient
		var totalAmount float64

		for rows.Next() {
			var inv InvoiceWithClient
			if err := rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.IssueDate, &inv.DueDate,
				&inv.TotalAmount, &inv.Status, &inv.ClientName); err != nil {
				return nil, nil, fmt.Errorf("failed to scan invoice: %w", err)
			}
			invoices = append(invoices, inv)
			totalAmount += inv.TotalAmount
		}

		text := fmt.Sprintf("Found %d invoices (Total: $%.2f):\n", len(invoices), totalAmount)
		for _, inv := range invoices {
			text += fmt.Sprintf("- %s: %s - $%.2f (%s) - Due: %s\n",
				inv.InvoiceNumber, inv.ClientName, inv.TotalAmount, inv.Status,
				inv.DueDate.Format("2006-01-02"))
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, map[string]interface{}{
			"invoices": invoices,
			"total_amount": totalAmount,
			"count": len(invoices),
		}, nil
	})
}

type Handler struct {
	db *sql.DB
}

func (h *Handler) getClientIDByName(name string) (int, error) {
	var id int
	err := h.db.QueryRow("SELECT id FROM clients WHERE name = ?", name).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("client '%s' not found", name)
	}
	return id, err
}