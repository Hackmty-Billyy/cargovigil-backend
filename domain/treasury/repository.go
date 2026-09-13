package treasury

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

// Method names are suffixed per entity (CreateInvoice, CreateExpense, ...)
// rather than the bare Create/GetByID/Update used elsewhere in this codebase,
// because a single BunRepository struct implements all five interfaces
// below on purpose (payments span invoices/expenses AND bank_accounts in one
// transaction) and Go does not allow two methods named Create on one type.

type BankAccountRepository interface {
	CreateBankAccount(ctx context.Context, a *BankAccount) error
	GetBankAccountByID(ctx context.Context, companyID, id string) (*BankAccount, error)
	ListBankAccountsByCompany(ctx context.Context, companyID string) ([]BankAccount, error)
	UpdateBankAccount(ctx context.Context, a *BankAccount) error
	DeleteBankAccount(ctx context.Context, companyID, id string) error
}

type InvoiceRepository interface {
	CreateInvoice(ctx context.Context, inv *Invoice) error
	GetInvoiceByID(ctx context.Context, companyID, id string) (*Invoice, error)
	ListInvoicesByCompany(ctx context.Context, companyID string) ([]Invoice, error)
	ListUnpaidInvoicesByCompany(ctx context.Context, companyID string) ([]Invoice, error)
	UpdateInvoice(ctx context.Context, inv *Invoice) error
	DeleteInvoice(ctx context.Context, companyID, id string) error
	RecordPayment(ctx context.Context, companyID, id string, amount float64, bankAccountID string) (*Invoice, error)
}

type ExpenseRepository interface {
	CreateExpense(ctx context.Context, e *Expense) error
	GetExpenseByID(ctx context.Context, companyID, id string) (*Expense, error)
	ListExpensesByCompany(ctx context.Context, companyID string) ([]Expense, error)
	ListPendingExpensesByCompany(ctx context.Context, companyID string) ([]Expense, error)
	UpdateExpense(ctx context.Context, e *Expense) error
	DeleteExpense(ctx context.Context, companyID, id string) error
	MarkPaid(ctx context.Context, companyID, id, bankAccountID string) (*Expense, error)
}

type CashAlertRepository interface {
	CreateAlert(ctx context.Context, a *CashAlert) error
	UpdateAlert(ctx context.Context, a *CashAlert) error
	GetOpenAlertByCompanyAndDate(ctx context.Context, companyID string, date time.Time) (*CashAlert, error)
	ListAlertsByCompany(ctx context.Context, companyID string, onlyUnresolved bool) ([]CashAlert, error)
	SetAlertResolved(ctx context.Context, companyID, id string, resolved bool) error
}

type CashFlowProjectionRepository interface {
	UpsertProjections(ctx context.Context, companyID string, rows []CashFlowProjection) error
	ListProjectionsByCompany(ctx context.Context, companyID string, days int) ([]CashFlowProjection, error)
}

// BunRepository implements all five interfaces above. They share one
// struct (not one-per-entity like domain/company) because recording a
// payment has to update an invoice/expense AND the bank account balance in
// the same transaction, and Bun's db.RunInTx makes that straightforward
// from one struct with access to every table involved.
type BunRepository struct{ db *bun.DB }

func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// ---- Bank accounts ----

func (r *BunRepository) CreateBankAccount(ctx context.Context, a *BankAccount) error {
	a.IsActive = true
	_, err := r.db.NewInsert().Model(a).
		Column("company_id", "bank_name", "account_number_mask", "currency", "current_balance", "minimum_required_balance", "is_active").
		Returning("id, is_active, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunRepository) GetBankAccountByID(ctx context.Context, companyID, id string) (*BankAccount, error) {
	a := new(BankAccount)
	err := r.db.NewSelect().Model(a).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *BunRepository) ListBankAccountsByCompany(ctx context.Context, companyID string) ([]BankAccount, error) {
	out := []BankAccount{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateBankAccount deliberately never touches current_balance — that field
// only ever moves through RecordPayment/MarkPaid, atomically with the
// invoice/expense it's tied to.
func (r *BunRepository) UpdateBankAccount(ctx context.Context, a *BankAccount) error {
	_, err := r.db.NewUpdate().Model(a).
		Set("bank_name = ?", a.BankName).
		Set("account_number_mask = ?", a.AccountNumberMask).
		Set("currency = ?", a.Currency).
		Set("minimum_required_balance = ?", a.MinimumRequiredBalance).
		Set("is_active = ?", a.IsActive).
		Set("updated_at = now()").
		Where("company_id = ?", a.CompanyID).
		Where("id = ?", a.ID).
		Exec(ctx)
	return err
}

func (r *BunRepository) DeleteBankAccount(ctx context.Context, companyID, id string) error {
	_, err := r.db.NewDelete().Model((*BankAccount)(nil)).Where("company_id = ?", companyID).Where("id = ?", id).Exec(ctx)
	return err
}

// ---- Invoices ----

func (r *BunRepository) CreateInvoice(ctx context.Context, inv *Invoice) error {
	inv.PaidAmount = 0
	inv.Status = "issued"
	_, err := r.db.NewInsert().Model(inv).
		Column("company_id", "trip_id", "client_id", "invoice_number", "issue_date", "due_date", "adjusted_due_date", "total_amount", "paid_amount", "status").
		Returning("id, paid_amount, status, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunRepository) GetInvoiceByID(ctx context.Context, companyID, id string) (*Invoice, error) {
	inv := new(Invoice)
	err := r.db.NewSelect().Model(inv).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *BunRepository) ListInvoicesByCompany(ctx context.Context, companyID string) ([]Invoice, error) {
	out := []Invoice{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("due_date ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) ListUnpaidInvoicesByCompany(ctx context.Context, companyID string) ([]Invoice, error) {
	out := []Invoice{}
	err := r.db.NewSelect().Model(&out).
		Where("company_id = ?", companyID).
		Where("status NOT IN (?)", bun.In([]string{"paid", "cancelled"})).
		OrderExpr("adjusted_due_date ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateInvoice never touches paid_amount/status/bank_account_id — those
// are only ever changed by RecordPayment.
func (r *BunRepository) UpdateInvoice(ctx context.Context, inv *Invoice) error {
	_, err := r.db.NewUpdate().Model(inv).
		Set("trip_id = ?", inv.TripID).
		Set("client_id = ?", inv.ClientID).
		Set("invoice_number = ?", inv.InvoiceNumber).
		Set("issue_date = ?", inv.IssueDate).
		Set("due_date = ?", inv.DueDate).
		Set("adjusted_due_date = ?", inv.AdjustedDueDate).
		Set("total_amount = ?", inv.TotalAmount).
		Set("updated_at = now()").
		Where("company_id = ?", inv.CompanyID).
		Where("id = ?", inv.ID).
		Exec(ctx)
	return err
}

func (r *BunRepository) DeleteInvoice(ctx context.Context, companyID, id string) error {
	_, err := r.db.NewDelete().Model((*Invoice)(nil)).Where("company_id = ?", companyID).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *BunRepository) RecordPayment(ctx context.Context, companyID, id string, amount float64, bankAccountID string) (*Invoice, error) {
	var result *Invoice
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		inv := new(Invoice)
		if err := tx.NewSelect().Model(inv).
			Where("company_id = ?", companyID).Where("id = ?", id).
			For("UPDATE").
			Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		newPaid := inv.PaidAmount + amount
		if newPaid > inv.TotalAmount+0.005 {
			return ErrOverpayment
		}
		status := "partially_paid"
		if newPaid >= inv.TotalAmount-0.005 {
			newPaid = inv.TotalAmount
			status = "paid"
		}

		if _, err := tx.NewUpdate().Model((*Invoice)(nil)).
			Set("paid_amount = ?", newPaid).
			Set("status = ?", status).
			Set("bank_account_id = ?", bankAccountID).
			Set("updated_at = now()").
			Where("id = ?", id).
			Exec(ctx); err != nil {
			return err
		}

		res, err := tx.NewUpdate().Model((*BankAccount)(nil)).
			Set("current_balance = current_balance + ?", amount).
			Set("updated_at = now()").
			Where("company_id = ?", companyID).Where("id = ?", bankAccountID).
			Exec(ctx)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}

		inv.PaidAmount = newPaid
		inv.Status = status
		inv.BankAccountID = &bankAccountID
		result = inv
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ---- Expenses ----

func (r *BunRepository) CreateExpense(ctx context.Context, e *Expense) error {
	e.Status = "pending"
	_, err := r.db.NewInsert().Model(e).
		Column("company_id", "trip_id", "category", "description", "amount", "due_date", "status").
		Returning("id, status, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunRepository) GetExpenseByID(ctx context.Context, companyID, id string) (*Expense, error) {
	e := new(Expense)
	err := r.db.NewSelect().Model(e).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (r *BunRepository) ListExpensesByCompany(ctx context.Context, companyID string) ([]Expense, error) {
	out := []Expense{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("due_date ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) ListPendingExpensesByCompany(ctx context.Context, companyID string) ([]Expense, error) {
	out := []Expense{}
	err := r.db.NewSelect().Model(&out).
		Where("company_id = ?", companyID).
		Where("status = ?", "pending").
		OrderExpr("due_date ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateExpense never touches paid_date/status/bank_account_id — those are
// only ever changed by MarkPaid.
func (r *BunRepository) UpdateExpense(ctx context.Context, e *Expense) error {
	_, err := r.db.NewUpdate().Model(e).
		Set("trip_id = ?", e.TripID).
		Set("category = ?", e.Category).
		Set("description = ?", e.Description).
		Set("amount = ?", e.Amount).
		Set("due_date = ?", e.DueDate).
		Set("updated_at = now()").
		Where("company_id = ?", e.CompanyID).
		Where("id = ?", e.ID).
		Exec(ctx)
	return err
}

func (r *BunRepository) DeleteExpense(ctx context.Context, companyID, id string) error {
	_, err := r.db.NewDelete().Model((*Expense)(nil)).Where("company_id = ?", companyID).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *BunRepository) MarkPaid(ctx context.Context, companyID, id, bankAccountID string) (*Expense, error) {
	var result *Expense
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		e := new(Expense)
		if err := tx.NewSelect().Model(e).
			Where("company_id = ?", companyID).Where("id = ?", id).
			For("UPDATE").
			Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if e.Status == "paid" {
			return ErrInvalidStatus
		}

		if _, err := tx.NewUpdate().Model((*Expense)(nil)).
			Set("status = ?", "paid").
			Set("paid_date = now()").
			Set("bank_account_id = ?", bankAccountID).
			Set("updated_at = now()").
			Where("id = ?", id).
			Exec(ctx); err != nil {
			return err
		}

		res, err := tx.NewUpdate().Model((*BankAccount)(nil)).
			Set("current_balance = current_balance - ?", e.Amount).
			Set("updated_at = now()").
			Where("company_id = ?", companyID).Where("id = ?", bankAccountID).
			Exec(ctx)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}

		e.Status = "paid"
		e.BankAccountID = &bankAccountID
		result = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ---- Cash alerts ----

func (r *BunRepository) CreateAlert(ctx context.Context, a *CashAlert) error {
	a.IsResolved = false
	_, err := r.db.NewInsert().Model(a).
		Column("company_id", "projected_date", "severity", "projected_deficit", "description", "is_resolved").
		Returning("id, is_resolved, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunRepository) UpdateAlert(ctx context.Context, a *CashAlert) error {
	_, err := r.db.NewUpdate().Model(a).
		Set("severity = ?", a.Severity).
		Set("projected_deficit = ?", a.ProjectedDeficit).
		Set("description = ?", a.Description).
		Set("updated_at = now()").
		Where("id = ?", a.ID).
		Exec(ctx)
	return err
}

// GetOpenAlertByCompanyAndDate returns (nil, nil) — not an error — when
// there is no open alert for that day, matching the "no row" case the
// caller (raiseOrUpdateAlert) treats as "create a new one".
func (r *BunRepository) GetOpenAlertByCompanyAndDate(ctx context.Context, companyID string, date time.Time) (*CashAlert, error) {
	a := new(CashAlert)
	err := r.db.NewSelect().Model(a).
		Where("company_id = ?", companyID).
		Where("projected_date = ?", date).
		Where("is_resolved = false").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *BunRepository) ListAlertsByCompany(ctx context.Context, companyID string, onlyUnresolved bool) ([]CashAlert, error) {
	out := []CashAlert{}
	q := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID)
	if onlyUnresolved {
		q = q.Where("is_resolved = false")
	}
	if err := q.OrderExpr("projected_date ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) SetAlertResolved(ctx context.Context, companyID, id string, resolved bool) error {
	_, err := r.db.NewUpdate().Model((*CashAlert)(nil)).
		Set("is_resolved = ?", resolved).
		Set("updated_at = now()").
		Where("company_id = ?", companyID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// ---- Cash flow projections ----

// UpsertProjections used to loop individual upserts inside a manual
// transaction; Bun's multi-row insert does the same ON CONFLICT upsert for
// the whole batch in one statement, which is already atomic on its own.
func (r *BunRepository) UpsertProjections(ctx context.Context, companyID string, rows []CashFlowProjection) error {
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].CompanyID = companyID
		rows[i].GeneratedAt = time.Time{} // let the DB default (now()) fill it
	}
	_, err := r.db.NewInsert().Model(&rows).
		On("CONFLICT (company_id, projected_date) DO UPDATE").
		Set("projected_balance = EXCLUDED.projected_balance").
		Set("generated_at = now()").
		Exec(ctx)
	return err
}

func (r *BunRepository) ListProjectionsByCompany(ctx context.Context, companyID string, days int) ([]CashFlowProjection, error) {
	out := []CashFlowProjection{}
	err := r.db.NewSelect().Model(&out).
		Where("company_id = ?", companyID).
		Where("projected_date <= CURRENT_DATE + ?::int", days).
		OrderExpr("projected_date ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}
