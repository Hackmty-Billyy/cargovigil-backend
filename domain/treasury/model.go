package treasury

import (
	"time"

	"github.com/uptrace/bun"
)

// bun:"..." tags map these onto the tables from migrations 000012/000018 —
// see domain/fuel/model.go for the convention. cash_flow_projections is the
// one hypertable in this package (000019), so its primary key includes the
// time column like fuel's FuelIndex/TripFuelLog.

type BankAccount struct {
	bun.BaseModel `bun:"table:bank_accounts,alias:ba"`

	ID                     string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID              string    `bun:"company_id,notnull" json:"company_id"`
	BankName               string    `bun:"bank_name,notnull" json:"bank_name"`
	AccountNumberMask      string    `bun:"account_number_mask,notnull" json:"account_number_mask"`
	Currency               string    `bun:"currency,notnull" json:"currency"`
	CurrentBalance         float64   `bun:"current_balance,notnull" json:"current_balance"`
	MinimumRequiredBalance float64   `bun:"minimum_required_balance,notnull" json:"minimum_required_balance"`
	IsActive               bool      `bun:"is_active,notnull" json:"is_active"`
	CreatedAt              time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt              time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

type Invoice struct {
	bun.BaseModel `bun:"table:invoices,alias:inv"`

	ID              string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID       string    `bun:"company_id,notnull" json:"company_id"`
	TripID          *string   `bun:"trip_id" json:"trip_id"`
	ClientID        string    `bun:"client_id,notnull" json:"client_id"`
	BankAccountID   *string   `bun:"bank_account_id" json:"bank_account_id"`
	InvoiceNumber   string    `bun:"invoice_number,notnull" json:"invoice_number"`
	IssueDate       time.Time `bun:"issue_date,type:date,notnull" json:"issue_date"`
	DueDate         time.Time `bun:"due_date,type:date,notnull" json:"due_date"`
	AdjustedDueDate time.Time `bun:"adjusted_due_date,type:date,notnull" json:"adjusted_due_date"`
	TotalAmount     float64   `bun:"total_amount,notnull" json:"total_amount"`
	PaidAmount      float64   `bun:"paid_amount,notnull" json:"paid_amount"`
	Status          string    `bun:"status,notnull" json:"status"`
	CreatedAt       time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt       time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

type Expense struct {
	bun.BaseModel `bun:"table:expenses,alias:e"`

	ID            string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID     string     `bun:"company_id,notnull" json:"company_id"`
	TripID        *string    `bun:"trip_id" json:"trip_id"`
	BankAccountID *string    `bun:"bank_account_id" json:"bank_account_id"`
	Category      string     `bun:"category,notnull" json:"category"`
	Description   string     `bun:"description,notnull" json:"description"`
	Amount        float64    `bun:"amount,notnull" json:"amount"`
	DueDate       time.Time  `bun:"due_date,type:date,notnull" json:"due_date"`
	PaidDate      *time.Time `bun:"paid_date,type:date" json:"paid_date"`
	Status        string     `bun:"status,notnull" json:"status"`
	CreatedAt     time.Time  `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt     time.Time  `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

type CashAlert struct {
	bun.BaseModel `bun:"table:cash_alerts,alias:ca"`

	ID               string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID        string    `bun:"company_id,notnull" json:"company_id"`
	ProjectedDate    time.Time `bun:"projected_date,type:date,notnull" json:"projected_date"`
	Severity         string    `bun:"severity,notnull" json:"severity"`
	ProjectedDeficit float64   `bun:"projected_deficit,notnull" json:"projected_deficit"`
	Description      string    `bun:"description,notnull" json:"description"`
	IsResolved       bool      `bun:"is_resolved,notnull" json:"is_resolved"`
	CreatedAt        time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt        time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

// CashFlowProjection lives on a hypertable (000019): PK widened to
// (id, projected_date), so both are tagged `,pk`.
type CashFlowProjection struct {
	bun.BaseModel `bun:"table:cash_flow_projections,alias:cfp"`

	ID               string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID        string    `bun:"company_id,notnull" json:"company_id"`
	ProjectedDate    time.Time `bun:"projected_date,pk,type:date,notnull" json:"projected_date"`
	ProjectedBalance float64   `bun:"projected_balance,notnull" json:"projected_balance"`
	GeneratedAt      time.Time `bun:"generated_at,nullzero,default:now()" json:"generated_at"`
}
