package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Initialize() (*sql.DB, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	dbPath := filepath.Join(homeDir, ".hours", "db")
	dbDir := filepath.Dir(dbPath)

	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := createTables(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		address TEXT,
		city TEXT,
		state TEXT,
		zip_code TEXT,
		country TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS recipients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		title TEXT,
		phone TEXT,
		is_primary BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS payment_details (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id INTEGER NOT NULL UNIQUE,
		bank_name TEXT,
		account_number TEXT,
		routing_number TEXT,
		swift_code TEXT,
		payment_terms TEXT,
		notes TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS time_entries (
		id TEXT PRIMARY KEY,
		client_id INTEGER NOT NULL,
		date DATE NOT NULL,
		hours REAL NOT NULL,
		description TEXT,
		contract_ref TEXT,
		invoice_id INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
		FOREIGN KEY (invoice_id) REFERENCES invoices(id)
	);

	CREATE TABLE IF NOT EXISTS invoices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id INTEGER NOT NULL,
		invoice_number TEXT NOT NULL UNIQUE,
		issue_date DATE NOT NULL,
		due_date DATE NOT NULL,
		total_amount REAL NOT NULL,
		status TEXT DEFAULT 'pending',
		pdf_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS contracts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id INTEGER NOT NULL,
		contract_number TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		hourly_rate REAL NOT NULL,
		currency TEXT DEFAULT 'USD',
		contract_type TEXT DEFAULT 'hourly',
		start_date DATE NOT NULL,
		end_date DATE,
		status TEXT DEFAULT 'active',
		payment_terms TEXT,
		notes TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS business_info (
		id INTEGER PRIMARY KEY,
		business_name TEXT NOT NULL,
		contact_name TEXT NOT NULL,
		email TEXT NOT NULL,
		phone TEXT,
		address TEXT,
		city TEXT,
		state TEXT,
		zip_code TEXT,
		country TEXT,
		tax_id TEXT,
		website TEXT,
		logo_path TEXT,
		invoice_prefix TEXT DEFAULT 'INV',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_time_entries_date ON time_entries(date);
	CREATE INDEX IF NOT EXISTS idx_time_entries_client ON time_entries(client_id);
	CREATE INDEX IF NOT EXISTS idx_invoices_client ON invoices(client_id);
	CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
	CREATE INDEX IF NOT EXISTS idx_contracts_client ON contracts(client_id);
	CREATE INDEX IF NOT EXISTS idx_contracts_status ON contracts(status);
	CREATE INDEX IF NOT EXISTS idx_contracts_dates ON contracts(start_date, end_date);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

func runMigrations(db *sql.DB) error {
	// Create migrations table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS migrations (
			id INTEGER PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	migrations := []migration{
		{
			name: "add_contract_ref_to_time_entries",
			apply: func(db *sql.DB) error {
				return addColumnIfNotExists(db, "time_entries", "contract_ref", "TEXT")
			},
		},
		{
			name: "add_title_to_recipients",
			apply: func(db *sql.DB) error {
				return addColumnIfNotExists(db, "recipients", "title", "TEXT")
			},
		},
		{
			name: "add_phone_to_recipients",
			apply: func(db *sql.DB) error {
				return addColumnIfNotExists(db, "recipients", "phone", "TEXT")
			},
		},
		{
			name: "add_address_to_clients",
			apply: func(db *sql.DB) error {
				if err := addColumnIfNotExists(db, "clients", "address", "TEXT"); err != nil {
					return err
				}
				if err := addColumnIfNotExists(db, "clients", "city", "TEXT"); err != nil {
					return err
				}
				if err := addColumnIfNotExists(db, "clients", "state", "TEXT"); err != nil {
					return err
				}
				if err := addColumnIfNotExists(db, "clients", "zip_code", "TEXT"); err != nil {
					return err
				}
				return addColumnIfNotExists(db, "clients", "country", "TEXT")
			},
		},
		{
			name: "restructure_for_contracts",
			apply: func(db *sql.DB) error {
				return restructureForContracts(db)
			},
		},
		{
			name: "remove_rate_constraints_from_clients",
			apply: func(db *sql.DB) error {
				return removeRateConstraintsFromClients(db)
			},
		},
	}

	for _, migration := range migrations {
		// Check if migration has already been applied
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM migrations WHERE name = ?", migration.name).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", migration.name, err)
		}

		if count > 0 {
			// Migration already applied
			continue
		}

		// Apply migration
		err = migration.apply(db)
		if err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.name, err)
		}

		// Record migration as applied
		_, err = db.Exec("INSERT INTO migrations (name) VALUES (?)", migration.name)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", migration.name, err)
		}

		fmt.Printf("Applied migration: %s\n", migration.name)
	}

	return nil
}

type migration struct {
	name  string
	apply func(*sql.DB) error
}

func addColumnIfNotExists(db *sql.DB, tableName, columnName, columnType string) error {
	// Check if column already exists
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return fmt.Errorf("failed to get table info for %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue *string
		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey)
		if err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}

		if name == columnName {
			// Column already exists
			fmt.Printf("Column %s.%s already exists, skipping\n", tableName, columnName)
			return nil
		}
	}

	// Column doesn't exist, add it
	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnType)
	_, err = db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to add column %s to %s: %w", columnName, tableName, err)
	}

	fmt.Printf("Added column %s.%s\n", tableName, columnName)
	return nil
}

func restructureForContracts(db *sql.DB) error {
	fmt.Println("Restructuring database for contract-based billing...")

	// Step 1: Create contracts table if it doesn't exist (will be created by main schema)
	// The contracts table is already in the main schema above

	// Step 2: Add contract_id column to time_entries
	if err := addColumnIfNotExists(db, "time_entries", "contract_id", "INTEGER"); err != nil {
		return fmt.Errorf("failed to add contract_id to time_entries: %w", err)
	}

	// Step 3: Check if we have any existing clients with rates that need migration
	var clientCount int
	err := db.QueryRow("SELECT COUNT(*) FROM clients WHERE hourly_rate IS NOT NULL AND hourly_rate > 0").Scan(&clientCount)
	if err != nil {
		return fmt.Errorf("failed to check existing clients: %w", err)
	}

	if clientCount > 0 {
		fmt.Printf("Migrating %d clients to contract-based structure...\n", clientCount)

		// Step 4: Create default contracts for existing clients
		rows, err := db.Query(`
			SELECT id, name, hourly_rate, currency, created_at
			FROM clients
			WHERE hourly_rate IS NOT NULL AND hourly_rate > 0
		`)
		if err != nil {
			return fmt.Errorf("failed to query existing clients: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var clientID int
			var clientName string
			var hourlyRate float64
			var currency string
			var createdAt string

			err := rows.Scan(&clientID, &clientName, &hourlyRate, &currency, &createdAt)
			if err != nil {
				return fmt.Errorf("failed to scan client: %w", err)
			}

			// Create a default contract for this client
			contractNumber := fmt.Sprintf("LEGACY-%d", clientID)
			contractName := fmt.Sprintf("Legacy Contract - %s", clientName)

			var contractID int64
			err = db.QueryRow(`
				INSERT INTO contracts (client_id, contract_number, name, hourly_rate, currency, start_date, status)
				VALUES (?, ?, ?, ?, ?, ?, 'active')
				RETURNING id
			`, clientID, contractNumber, contractName, hourlyRate, currency, createdAt[:10]).Scan(&contractID)

			if err != nil {
				return fmt.Errorf("failed to create legacy contract for client %s: %w", clientName, err)
			}

			// Step 5: Update existing time entries to reference the new contract
			_, err = db.Exec(`
				UPDATE time_entries
				SET contract_id = ?
				WHERE client_id = ? AND contract_id IS NULL
			`, contractID, clientID)

			if err != nil {
				return fmt.Errorf("failed to update time entries for client %s: %w", clientName, err)
			}

			fmt.Printf("Created legacy contract %s for client %s\n", contractNumber, clientName)
		}
	}

	// Step 6: Make contract_id required and add foreign key constraint for new time entries
	// We'll handle this in business logic rather than database constraints for easier migration

	fmt.Println("Contract restructuring completed successfully!")
	return nil
}

func removeRateConstraintsFromClients(db *sql.DB) error {
	fmt.Println("Removing rate constraints from clients table...")

	// SQLite doesn't support ALTER TABLE DROP COLUMN or modifying constraints directly
	// We need to recreate the table without the rate fields

	// Step 1: Create new clients table without rate fields
	_, err := db.Exec(`
		CREATE TABLE clients_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			address TEXT,
			city TEXT,
			state TEXT,
			zip_code TEXT,
			country TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create new clients table: %w", err)
	}

	// Step 2: Copy data from old table (excluding rate fields)
	_, err = db.Exec(`
		INSERT INTO clients_new (id, name, address, city, state, zip_code, country, created_at, updated_at)
		SELECT id, name, address, city, state, zip_code, country, created_at, updated_at
		FROM clients
	`)
	if err != nil {
		return fmt.Errorf("failed to copy client data: %w", err)
	}

	// Step 3: Drop old table and rename new table
	_, err = db.Exec(`DROP TABLE clients`)
	if err != nil {
		return fmt.Errorf("failed to drop old clients table: %w", err)
	}

	_, err = db.Exec(`ALTER TABLE clients_new RENAME TO clients`)
	if err != nil {
		return fmt.Errorf("failed to rename new clients table: %w", err)
	}

	fmt.Println("Successfully removed rate constraints from clients table")
	return nil
}
