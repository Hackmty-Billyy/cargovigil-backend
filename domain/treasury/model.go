package treasury

import "time"

type BankAccount struct {
	ID                     string    `json:"id"`
	CompanyID              string    `json:"company_id"`
	BankName               string    `json:"bank_name"`
	AccountNumberMask      string    `json:"account_number_mask"`
	Currency               string    `json:"currency"`
	CurrentBalance         float64   `json:"current_balance"`
	MinimumRequiredBalance float64   `json:"minimum_required_balance"`
	IsActive               bool      `json:"is_active"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type Invoice struct {
	ID              string    `json:"id"`
	CompanyID       string    `json:"company_id"`
	TripID          *string   `json:"trip_id"`
	ClientID        string    `json:"client_id"`
	BankAccountID   *string   `json:"bank_account_id"`
	InvoiceNumber   string    `json:"invoice_number"`
	IssueDate       time.Time `json:"issue_date"`
	DueDate         time.Time `json:"due_date"`
	AdjustedDueDate time.Time `json:"adjusted_due_date"`
	TotalAmount     float64   `json:"total_amount"`
	PaidAmount      float64   `json:"paid_amount"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Expense struct {
	ID            string     `json:"id"`
	CompanyID     string     `json:"company_id"`
	TripID        *string    `json:"trip_id"`
	BankAccountID *string    `json:"bank_account_id"`
	Category      string     `json:"category"`
	Description   string     `json:"description"`
	Amount        float64    `json:"amount"`
	DueDate       time.Time  `json:"due_date"`
	PaidDate      *time.Time `json:"paid_date"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type CashAlert struct {
	ID               string    `json:"id"`
	CompanyID        string    `json:"company_id"`
	ProjectedDate    time.Time `json:"projected_date"`
	Severity         string    `json:"severity"`
	ProjectedDeficit float64   `json:"projected_deficit"`
	Description      string    `json:"description"`
	IsResolved       bool      `json:"is_resolved"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CashFlowProjection struct {
	ID               string    `json:"id"`
	CompanyID        string    `json:"company_id"`
	ProjectedDate    time.Time `json:"projected_date"`
	ProjectedBalance float64   `json:"projected_balance"`
	GeneratedAt      time.Time `json:"generated_at"`
}
