package pdf

import (
	"fmt"

	"github.com/austin/hours-mcp/internal/models"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

type InvoiceGenerator struct{}

func NewInvoiceGenerator() *InvoiceGenerator {
	return &InvoiceGenerator{}
}

func (g *InvoiceGenerator) Generate(invoice models.Invoice, payment models.PaymentDetails, recipients []models.Recipient, business models.BusinessInfo, outputPath string) error {
	// For contract-based billing, we need to group entries by contract and calculate rates per contract
	contractGroups := make(map[int][]models.TimeEntry)
	contractInfo := make(map[int]models.Contract)

	// Group time entries by contract
	for _, entry := range invoice.TimeEntries {
		if entry.Contract != nil {
			contractGroups[entry.ContractID] = append(contractGroups[entry.ContractID], entry)
			contractInfo[entry.ContractID] = *entry.Contract
		}
	}
	m := maroto.New(config.NewBuilder().Build())

	// Business Header
	m.AddRow(10,
		col.New(8).Add(
			text.New(business.BusinessName, props.Text{
				Size:  16,
				Style: fontstyle.Bold,
			}),
		),
		col.New(4).Add(
			text.New("INVOICE", props.Text{
				Size:  20,
				Style: fontstyle.BoldItalic,
				Align: align.Right,
			}),
		),
	)

	// Business contact info and invoice details
	m.AddRow(6,
		col.New(8).Add(
			text.New(business.ContactName, props.Text{
				Size: 10,
			}),
		),
		col.New(4).Add(
			text.New(fmt.Sprintf("Invoice #: %s", invoice.InvoiceNumber), props.Text{
				Size:  10,
				Style: fontstyle.Bold,
				Align: align.Right,
			}),
		),
	)

	if business.Email != "" {
		m.AddRow(5,
			col.New(8).Add(
				text.New(business.Email, props.Text{
					Size: 9,
				}),
			),
			col.New(4).Add(
				text.New(fmt.Sprintf("Date: %s", invoice.IssueDate.Format("January 2, 2006")), props.Text{
					Size:  9,
					Align: align.Right,
				}),
			),
		)
	}

	if business.Phone != "" {
		m.AddRow(5,
			col.New(8).Add(
				text.New(business.Phone, props.Text{
					Size: 9,
				}),
			),
			col.New(4).Add(
				text.New(fmt.Sprintf("Due Date: %s", invoice.DueDate.Format("January 2, 2006")), props.Text{
					Size: 9,
					Align: align.Right,
				}),
			),
		)
	} else {
		m.AddRow(5,
			col.New(8),
			col.New(4).Add(
				text.New(fmt.Sprintf("Due Date: %s", invoice.DueDate.Format("January 2, 2006")), props.Text{
					Size: 9,
					Align: align.Right,
				}),
			),
		)
	}

	// Business address if available
	if business.Address != "" {
		addressText := business.Address
		if business.City != "" {
			addressText += ", " + business.City
		}
		if business.State != "" {
			addressText += ", " + business.State
		}
		if business.ZipCode != "" {
			addressText += " " + business.ZipCode
		}
		if business.Country != "" {
			addressText += ", " + business.Country
		}

		m.AddRow(5,
			col.New(8).Add(
				text.New(addressText, props.Text{
					Size: 9,
				}),
			),
		)
	}

	if business.Website != "" {
		m.AddRow(5,
			col.New(8).Add(
				text.New(business.Website, props.Text{
					Size: 9,
				}),
			),
		)
	}

	m.AddRow(10)

	m.AddRow(8,
		col.New(12).Add(
			text.New(fmt.Sprintf("Bill To: %s", invoice.Client.Name), props.Text{
				Size:  11,
				Style: fontstyle.Bold,
			}),
		),
	)

	// Add client address if available
	if invoice.Client.Address != "" {
		m.AddRow(5,
			col.New(12).Add(
				text.New(invoice.Client.Address, props.Text{
					Size: 9,
				}),
			),
		)
	}

	// Add city, state, zip if available
	if invoice.Client.City != "" || invoice.Client.State != "" || invoice.Client.ZipCode != "" {
		cityStateZip := ""
		if invoice.Client.City != "" {
			cityStateZip = invoice.Client.City
		}
		if invoice.Client.State != "" {
			if cityStateZip != "" {
				cityStateZip += ", "
			}
			cityStateZip += invoice.Client.State
		}
		if invoice.Client.ZipCode != "" {
			if cityStateZip != "" {
				cityStateZip += " "
			}
			cityStateZip += invoice.Client.ZipCode
		}

		m.AddRow(5,
			col.New(12).Add(
				text.New(cityStateZip, props.Text{
					Size: 9,
				}),
			),
		)
	}

	// Add country if available
	if invoice.Client.Country != "" {
		m.AddRow(5,
			col.New(12).Add(
				text.New(invoice.Client.Country, props.Text{
					Size: 9,
				}),
			),
		)
	}

	if len(recipients) > 0 {
		for _, r := range recipients {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("%s <%s>", r.Name, r.Email), props.Text{
						Size: 9,
					}),
				),
			)
		}
	}

	m.AddRow(10)

	// Add contract information (assuming single contract per invoice)
	if len(invoice.TimeEntries) > 0 && invoice.TimeEntries[0].Contract != nil {
		contract := invoice.TimeEntries[0].Contract

		m.AddRow(8,
			col.New(12).Add(
				text.New(fmt.Sprintf("Contract: %s - %s", contract.ContractNumber, contract.Name), props.Text{
					Size:  11,
					Style: fontstyle.Bold,
				}),
			),
		)

		m.AddRow(5,
			col.New(12).Add(
				text.New(fmt.Sprintf("Rate: %s %.0f per hour", contract.Currency, contract.HourlyRate), props.Text{
					Size: 9,
				}),
			),
		)

		if contract.PaymentTerms != "" {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("Terms: %s", contract.PaymentTerms), props.Text{
						Size: 9,
					}),
				),
			)
		}

		m.AddRow(8)
	}

	m.AddRow(8,
		col.New(12).Add(
			text.New("Time Entries", props.Text{
				Size:  12,
				Style: fontstyle.Bold,
			}),
		),
	)

	m.AddRow(8,
		col.New(2).Add(
			text.New("Date", props.Text{
				Size:  9,
				Style: fontstyle.Bold,
			}),
		),
		col.New(6).Add(
			text.New("Description", props.Text{
				Size:  9,
				Style: fontstyle.Bold,
			}),
		),
		col.New(1).Add(
			text.New("Hours", props.Text{
				Size:  9,
				Style: fontstyle.Bold,
				Align: align.Right,
			}),
		),
		col.New(3).Add(
			text.New("Amount", props.Text{
				Size:  9,
				Style: fontstyle.Bold,
				Align: align.Right,
			}),
		),
	)

	var totalHours float64
	var totalAmount float64

	for _, entry := range invoice.TimeEntries {
		if entry.Contract == nil {
			continue // Skip entries without contract info
		}

		amount := entry.Hours * entry.Contract.HourlyRate
		totalHours += entry.Hours
		totalAmount += amount

		m.AddRow(6,
			col.New(2).Add(
				text.New(entry.Date.Format("2006-01-02"), props.Text{
					Size: 8,
				}),
			),
			col.New(6).Add(
				text.New(entry.Description, props.Text{
					Size: 8,
				}),
			),
			col.New(1).Add(
				text.New(fmt.Sprintf("%.2f", entry.Hours), props.Text{
					Size:  8,
					Align: align.Right,
				}),
			),
			col.New(3).Add(
				text.New(fmt.Sprintf("%s %.2f", entry.Contract.Currency, amount), props.Text{
					Size:  8,
					Align: align.Right,
				}),
			),
		)
	}

	m.AddRow(8)

	m.AddRow(8,
		col.New(6),
		col.New(2).Add(
			text.New("Total Hours:", props.Text{
				Size:  9,
				Style: fontstyle.Bold,
			}),
		),
		col.New(1).Add(
			text.New(fmt.Sprintf("%.2f", totalHours), props.Text{
				Size:  9,
				Style: fontstyle.Bold,
				Align: align.Right,
			}),
		),
		col.New(3).Add(
			text.New(fmt.Sprintf("%.2f", totalAmount), props.Text{
				Size:  10,
				Style: fontstyle.Bold,
				Align: align.Right,
			}),
		),
	)

	if payment.BankName != "" || payment.PaymentTerms != "" {
		m.AddRow(10)
		m.AddRow(8,
			col.New(12).Add(
				text.New("Payment Information", props.Text{
					Size:  12,
					Style: fontstyle.Bold,
				}),
			),
		)

		if payment.PaymentTerms != "" {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("Terms: %s", payment.PaymentTerms), props.Text{
						Size: 9,
					}),
				),
			)
		}

		if payment.BankName != "" {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("Bank: %s", payment.BankName), props.Text{
						Size: 9,
					}),
				),
			)
		}

		if payment.AccountNumber != "" {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("Account: %s", payment.AccountNumber), props.Text{
						Size: 9,
					}),
				),
			)
		}

		if payment.RoutingNumber != "" {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("Routing: %s", payment.RoutingNumber), props.Text{
						Size: 9,
					}),
				),
			)
		}

		if payment.SwiftCode != "" {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("SWIFT: %s", payment.SwiftCode), props.Text{
						Size: 9,
					}),
				),
			)
		}

		if payment.Notes != "" {
			m.AddRow(8,
				col.New(12).Add(
					text.New(payment.Notes, props.Text{
						Size: 8,
					}),
				),
			)
		}
	}

	document, err := m.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate PDF document: %w", err)
	}

	if err := document.Save(outputPath); err != nil {
		return fmt.Errorf("failed to save PDF: %w", err)
	}

	return nil
}