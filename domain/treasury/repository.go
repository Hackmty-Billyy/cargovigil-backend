package treasury

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Method names are suffixed per entity (CreateInvoice, CreateExpense, ...)
// rather than the bare Create/GetByID/Update used elsewhere in this codebase,
// because a single PostgresRepository struct implements all five interfaces
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

// PostgresRepository implements all five interfaces above. They share one
// struct (not one-per-entity like domain/auth) because recording a payment
// has to update an invoice/expense AND the bank account balance in the same
// transaction, and it's simpler to do that with direct pool access than to
// coordinate two separate repositories.
type PostgresRepository struct{ db *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// ---- Bank accounts ----

func (r *PostgresRepository) CreateBankAccount(ctx context.Context, a *BankAccount) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO bank_accounts (company_id, bank_name, account_number_mask, currency, current_balance, minimum_required_balance, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, true)
		 RETURNING id, is_active, created_at, updated_at`,
		a.CompanyID, a.BankName, a.AccountNumberMask, a.Currency, a.CurrentBalance, a.MinimumRequiredBalance,
	).Scan(&a.ID, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
}

func (r *PostgresRepository) GetBankAccountByID(ctx context.Context, companyID, id string) (*BankAccount, error) {
	a := &BankAccount{}
	err := r.db.QueryRow(ctx,
		`SELECT id, company_id, bank_name, account_number_mask, currency, current_balance, minimum_required_balance, is_active, created_at, updated_at
		 FROM bank_accounts WHERE company_id = $1 AND id = $2`, companyID, id,
	).Scan(&a.ID, &a.CompanyID, &a.BankName, &a.AccountNumberMask, &a.Currency, &a.CurrentBalance, &a.MinimumRequiredBalance, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *PostgresRepository) ListBankAccountsByCompany(ctx context.Context, companyID string) ([]BankAccount, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, company_id, bank_name, account_number_mask, currency, current_balance, minimum_required_balance, is_active, created_at, updated_at
		 FROM bank_accounts WHERE company_id = $1 ORDER BY created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BankAccount{}
	for rows.Next() {
		var a BankAccount
		if err := rows.Scan(&a.ID, &a.CompanyID, &a.BankName, &a.AccountNumberMask, &a.Currency, &a.CurrentBalance, &a.MinimumRequiredBalance, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateBankAccount(ctx context.Context, a *BankAccount) error {
	_, err := r.db.Exec(ctx,
		`UPDATE bank_accounts SET bank_name=$1, account_number_mask=$2, currency=$3, minimum_required_balance=$4, is_active=$5, updated_at=now()
		 WHERE company_id=$6 AND id=$7`,
		a.BankName, a.AccountNumberMask, a.Currency, a.MinimumRequiredBalance, a.IsActive, a.CompanyID, a.ID)
	return err
}

func (r *PostgresRepository) DeleteBankAccount(ctx context.Context, companyID, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM bank_accounts WHERE company_id = $1 AND id = $2`, companyID, id)
	return err
}

// ---- Invoices ----

const invoiceColumns = `id, company_id, trip_id, client_id, bank_account_id, invoice_number,
	issue_date, due_date, adjusted_due_date, total_amount, paid_amount, status, created_at, updated_at`

func scanInvoice(row pgx.Row) (*Invoice, error) {
	inv := &Invoice{}
	err := row.Scan(&inv.ID, &inv.CompanyID, &inv.TripID, &inv.ClientID, &inv.BankAccountID, &inv.InvoiceNumber,
		&inv.IssueDate, &inv.DueDate, &inv.AdjustedDueDate, &inv.TotalAmount, &inv.PaidAmount, &inv.Status, &inv.CreatedAt, &inv.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *PostgresRepository) CreateInvoice(ctx context.Context, inv *Invoice) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO invoices (company_id, trip_id, client_id, invoice_number, issue_date, due_date, adjusted_due_date, total_amount, paid_amount, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, 'issued')
		 RETURNING id, paid_amount, status, created_at, updated_at`,
		inv.CompanyID, inv.TripID, inv.ClientID, inv.InvoiceNumber, inv.IssueDate, inv.DueDate, inv.AdjustedDueDate, inv.TotalAmount,
	).Scan(&inv.ID, &inv.PaidAmount, &inv.Status, &inv.CreatedAt, &inv.UpdatedAt)
}

func (r *PostgresRepository) GetInvoiceByID(ctx context.Context, companyID, id string) (*Invoice, error) {
	row := r.db.QueryRow(ctx, `SELECT `+invoiceColumns+` FROM invoices WHERE company_id = $1 AND id = $2`, companyID, id)
	return scanInvoice(row)
}

func (r *PostgresRepository) ListInvoicesByCompany(ctx context.Context, companyID string) ([]Invoice, error) {
	rows, err := r.db.Query(ctx, `SELECT `+invoiceColumns+` FROM invoices WHERE company_id = $1 ORDER BY due_date ASC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListUnpaidInvoicesByCompany(ctx context.Context, companyID string) ([]Invoice, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+invoiceColumns+` FROM invoices WHERE company_id = $1 AND status NOT IN ('paid', 'cancelled') ORDER BY adjusted_due_date ASC`,
		companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateInvoice(ctx context.Context, inv *Invoice) error {
	_, err := r.db.Exec(ctx,
		`UPDATE invoices SET trip_id=$1, client_id=$2, invoice_number=$3, issue_date=$4, due_date=$5, adjusted_due_date=$6, total_amount=$7, updated_at=now()
		 WHERE company_id=$8 AND id=$9`,
		inv.TripID, inv.ClientID, inv.InvoiceNumber, inv.IssueDate, inv.DueDate, inv.AdjustedDueDate, inv.TotalAmount, inv.CompanyID, inv.ID)
	return err
}

func (r *PostgresRepository) DeleteInvoice(ctx context.Context, companyID, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM invoices WHERE company_id = $1 AND id = $2`, companyID, id)
	return err
}

func (r *PostgresRepository) RecordPayment(ctx context.Context, companyID, id string, amount float64, bankAccountID string) (*Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	inv, err := scanInvoice(tx.QueryRow(ctx, `SELECT `+invoiceColumns+` FROM invoices WHERE company_id = $1 AND id = $2 FOR UPDATE`, companyID, id))
	if err != nil {
		return nil, err
	}

	newPaid := inv.PaidAmount + amount
	if newPaid > inv.TotalAmount+0.005 {
		return nil, ErrOverpayment
	}
	status := "partially_paid"
	if newPaid >= inv.TotalAmount-0.005 {
		newPaid = inv.TotalAmount
		status = "paid"
	}

	if _, err := tx.Exec(ctx,
		`UPDATE invoices SET paid_amount=$1, status=$2, bank_account_id=$3, updated_at=now() WHERE id=$4`,
		newPaid, status, bankAccountID, id); err != nil {
		return nil, err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE bank_accounts SET current_balance = current_balance + $1, updated_at=now() WHERE company_id=$2 AND id=$3`,
		amount, companyID, bankAccountID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	inv.PaidAmount = newPaid
	inv.Status = status
	inv.BankAccountID = &bankAccountID
	return inv, nil
}

// ---- Expenses ----

const expenseColumns = `id, company_id, trip_id, bank_account_id, category, description, amount, due_date, paid_date, status, created_at, updated_at`

func scanExpense(row pgx.Row) (*Expense, error) {
	e := &Expense{}
	err := row.Scan(&e.ID, &e.CompanyID, &e.TripID, &e.BankAccountID, &e.Category, &e.Description, &e.Amount, &e.DueDate, &e.PaidDate, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (r *PostgresRepository) CreateExpense(ctx context.Context, e *Expense) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO expenses (company_id, trip_id, category, description, amount, due_date, status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		 RETURNING id, status, created_at, updated_at`,
		e.CompanyID, e.TripID, e.Category, e.Description, e.Amount, e.DueDate,
	).Scan(&e.ID, &e.Status, &e.CreatedAt, &e.UpdatedAt)
}

func (r *PostgresRepository) GetExpenseByID(ctx context.Context, companyID, id string) (*Expense, error) {
	row := r.db.QueryRow(ctx, `SELECT `+expenseColumns+` FROM expenses WHERE company_id = $1 AND id = $2`, companyID, id)
	return scanExpense(row)
}

func (r *PostgresRepository) ListExpensesByCompany(ctx context.Context, companyID string) ([]Expense, error) {
	rows, err := r.db.Query(ctx, `SELECT `+expenseColumns+` FROM expenses WHERE company_id = $1 ORDER BY due_date ASC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListPendingExpensesByCompany(ctx context.Context, companyID string) ([]Expense, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+expenseColumns+` FROM expenses WHERE company_id = $1 AND status = 'pending' ORDER BY due_date ASC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateExpense(ctx context.Context, e *Expense) error {
	_, err := r.db.Exec(ctx,
		`UPDATE expenses SET trip_id=$1, category=$2, description=$3, amount=$4, due_date=$5, updated_at=now()
		 WHERE company_id=$6 AND id=$7`,
		e.TripID, e.Category, e.Description, e.Amount, e.DueDate, e.CompanyID, e.ID)
	return err
}

func (r *PostgresRepository) DeleteExpense(ctx context.Context, companyID, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM expenses WHERE company_id = $1 AND id = $2`, companyID, id)
	return err
}

func (r *PostgresRepository) MarkPaid(ctx context.Context, companyID, id, bankAccountID string) (*Expense, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	e, err := scanExpense(tx.QueryRow(ctx, `SELECT `+expenseColumns+` FROM expenses WHERE company_id = $1 AND id = $2 FOR UPDATE`, companyID, id))
	if err != nil {
		return nil, err
	}
	if e.Status == "paid" {
		return nil, ErrInvalidStatus
	}

	if _, err := tx.Exec(ctx,
		`UPDATE expenses SET status='paid', paid_date=now(), bank_account_id=$1, updated_at=now() WHERE id=$2`,
		bankAccountID, id); err != nil {
		return nil, err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE bank_accounts SET current_balance = current_balance - $1, updated_at=now() WHERE company_id=$2 AND id=$3`,
		e.Amount, companyID, bankAccountID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	e.Status = "paid"
	e.BankAccountID = &bankAccountID
	return e, nil
}

// ---- Cash alerts ----

const cashAlertColumns = `id, company_id, projected_date, severity, projected_deficit, description, is_resolved, created_at, updated_at`

func scanCashAlert(row pgx.Row) (*CashAlert, error) {
	a := &CashAlert{}
	err := row.Scan(&a.ID, &a.CompanyID, &a.ProjectedDate, &a.Severity, &a.ProjectedDeficit, &a.Description, &a.IsResolved, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *PostgresRepository) CreateAlert(ctx context.Context, a *CashAlert) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO cash_alerts (company_id, projected_date, severity, projected_deficit, description, is_resolved)
		 VALUES ($1, $2, $3, $4, $5, false)
		 RETURNING id, is_resolved, created_at, updated_at`,
		a.CompanyID, a.ProjectedDate, a.Severity, a.ProjectedDeficit, a.Description,
	).Scan(&a.ID, &a.IsResolved, &a.CreatedAt, &a.UpdatedAt)
}

func (r *PostgresRepository) UpdateAlert(ctx context.Context, a *CashAlert) error {
	_, err := r.db.Exec(ctx,
		`UPDATE cash_alerts SET severity=$1, projected_deficit=$2, description=$3, updated_at=now() WHERE id=$4`,
		a.Severity, a.ProjectedDeficit, a.Description, a.ID)
	return err
}

func (r *PostgresRepository) GetOpenAlertByCompanyAndDate(ctx context.Context, companyID string, date time.Time) (*CashAlert, error) {
	a, err := scanCashAlert(r.db.QueryRow(ctx,
		`SELECT `+cashAlertColumns+` FROM cash_alerts WHERE company_id = $1 AND projected_date = $2 AND is_resolved = false LIMIT 1`,
		companyID, date))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *PostgresRepository) ListAlertsByCompany(ctx context.Context, companyID string, onlyUnresolved bool) ([]CashAlert, error) {
	query := `SELECT ` + cashAlertColumns + ` FROM cash_alerts WHERE company_id = $1`
	if onlyUnresolved {
		query += ` AND is_resolved = false`
	}
	query += ` ORDER BY projected_date ASC`

	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CashAlert{}
	for rows.Next() {
		a, err := scanCashAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) SetAlertResolved(ctx context.Context, companyID, id string, resolved bool) error {
	_, err := r.db.Exec(ctx,
		`UPDATE cash_alerts SET is_resolved = $1, updated_at = now() WHERE company_id = $2 AND id = $3`,
		resolved, companyID, id)
	return err
}

// ---- Cash flow projections ----

func (r *PostgresRepository) UpsertProjections(ctx context.Context, companyID string, rows []CashFlowProjection) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, p := range rows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO cash_flow_projections (company_id, projected_date, projected_balance, generated_at)
			 VALUES ($1, $2, $3, now())
			 ON CONFLICT (company_id, projected_date)
			 DO UPDATE SET projected_balance = EXCLUDED.projected_balance, generated_at = now()`,
			companyID, p.ProjectedDate, p.ProjectedBalance); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListProjectionsByCompany(ctx context.Context, companyID string, days int) ([]CashFlowProjection, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, company_id, projected_date, projected_balance, generated_at
		 FROM cash_flow_projections
		 WHERE company_id = $1 AND projected_date <= CURRENT_DATE + $2::int
		 ORDER BY projected_date ASC`,
		companyID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CashFlowProjection{}
	for rows.Next() {
		var p CashFlowProjection
		if err := rows.Scan(&p.ID, &p.CompanyID, &p.ProjectedDate, &p.ProjectedBalance, &p.GeneratedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
