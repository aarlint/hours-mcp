package models

import (
	"time"
)

type Client struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address,omitempty"`
	City      string    `json:"city,omitempty"`
	State     string    `json:"state,omitempty"`
	ZipCode   string    `json:"zip_code,omitempty"`
	Country   string    `json:"country,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Contract struct {
	ID             int       `json:"id"`
	ClientID       int       `json:"client_id"`
	ContractNumber string    `json:"contract_number"`
	Name           string    `json:"name"`
	HourlyRate     float64   `json:"hourly_rate"`
	Currency       string    `json:"currency"`
	ContractType   string    `json:"contract_type"`
	StartDate      time.Time `json:"start_date"`
	EndDate        *time.Time `json:"end_date,omitempty"`
	Status         string    `json:"status"`
	PaymentTerms   string    `json:"payment_terms,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	Client *Client `json:"client,omitempty"`
}

type Recipient struct {
	ID        int       `json:"id"`
	ClientID  int       `json:"client_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Title     string    `json:"title,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	IsPrimary bool      `json:"is_primary"`
	CreatedAt time.Time `json:"created_at"`
}

type PaymentDetails struct {
	ID            int       `json:"id"`
	ClientID      int       `json:"client_id"`
	BankName      string    `json:"bank_name,omitempty"`
	AccountNumber string    `json:"account_number,omitempty"`
	RoutingNumber string    `json:"routing_number,omitempty"`
	SwiftCode     string    `json:"swift_code,omitempty"`
	PaymentTerms  string    `json:"payment_terms,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type TimeEntry struct {
	ID          string    `json:"id"`
	ContractID  int       `json:"contract_id"`
	Date        time.Time `json:"date"`
	Hours       float64   `json:"hours"`
	Description string    `json:"description,omitempty"`
	InvoiceID   *int      `json:"invoice_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`

	Contract *Contract `json:"contract,omitempty"`
}

type Invoice struct {
	ID            int       `json:"id"`
	ClientID      int       `json:"client_id"`
	InvoiceNumber string    `json:"invoice_number"`
	IssueDate     time.Time `json:"issue_date"`
	DueDate       time.Time `json:"due_date"`
	TotalAmount   float64   `json:"total_amount"`
	Status        string    `json:"status"`
	PDFPath       string    `json:"pdf_path,omitempty"`
	CreatedAt     time.Time `json:"created_at"`

	Client      *Client      `json:"client,omitempty"`
	TimeEntries []TimeEntry  `json:"time_entries,omitempty"`
	Contracts   []Contract   `json:"contracts,omitempty"`
}

type BusinessInfo struct {
	ID            int       `json:"id"`
	BusinessName  string    `json:"business_name"`
	ContactName   string    `json:"contact_name"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone,omitempty"`
	Address       string    `json:"address,omitempty"`
	City          string    `json:"city,omitempty"`
	State         string    `json:"state,omitempty"`
	ZipCode       string    `json:"zip_code,omitempty"`
	Country       string    `json:"country,omitempty"`
	TaxID         string    `json:"tax_id,omitempty"`
	Website       string    `json:"website,omitempty"`
	LogoPath      string    `json:"logo_path,omitempty"`
	InvoicePrefix string    `json:"invoice_prefix,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}