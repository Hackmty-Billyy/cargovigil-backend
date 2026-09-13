package treasury

import (
	"context"
	"fmt"
	"time"
)

// ForecastHorizonDays is the widest window the daily job precomputes; the API
// lets callers ask for 30 or 60 within this range.
const ForecastHorizonDays = 60

// forecastCurrency is the only currency the forecast engine understands
// right now. See the comment in RecalculateForecast.
const forecastCurrency = "MXN"

type Service struct {
	bankAccounts BankAccountRepository
	invoices     InvoiceRepository
	expenses     ExpenseRepository
	alerts       CashAlertRepository
	projections  CashFlowProjectionRepository
}

func NewService(bankAccounts BankAccountRepository, invoices InvoiceRepository, expenses ExpenseRepository,
	alerts CashAlertRepository, projections CashFlowProjectionRepository) *Service {
	return &Service{
		bankAccounts: bankAccounts,
		invoices:     invoices,
		expenses:     expenses,
		alerts:       alerts,
		projections:  projections,
	}
}

// ---- Bank accounts ----

func (s *Service) CreateBankAccount(ctx context.Context, a *BankAccount) error {
	return s.bankAccounts.CreateBankAccount(ctx, a)
}

func (s *Service) GetBankAccount(ctx context.Context, companyID, id string) (*BankAccount, error) {
	return s.bankAccounts.GetBankAccountByID(ctx, companyID, id)
}

func (s *Service) ListBankAccounts(ctx context.Context, companyID string) ([]BankAccount, error) {
	return s.bankAccounts.ListBankAccountsByCompany(ctx, companyID)
}

func (s *Service) UpdateBankAccount(ctx context.Context, a *BankAccount) error {
	return s.bankAccounts.UpdateBankAccount(ctx, a)
}

func (s *Service) DeleteBankAccount(ctx context.Context, companyID, id string) error {
	return s.bankAccounts.DeleteBankAccount(ctx, companyID, id)
}

// ---- Invoices ----

func (s *Service) CreateInvoice(ctx context.Context, inv *Invoice) error {
	return s.invoices.CreateInvoice(ctx, inv)
}

func (s *Service) GetInvoice(ctx context.Context, companyID, id string) (*Invoice, error) {
	return s.invoices.GetInvoiceByID(ctx, companyID, id)
}

func (s *Service) ListInvoices(ctx context.Context, companyID string) ([]Invoice, error) {
	return s.invoices.ListInvoicesByCompany(ctx, companyID)
}

func (s *Service) UpdateInvoice(ctx context.Context, inv *Invoice) error {
	return s.invoices.UpdateInvoice(ctx, inv)
}

func (s *Service) DeleteInvoice(ctx context.Context, companyID, id string) error {
	return s.invoices.DeleteInvoice(ctx, companyID, id)
}

func (s *Service) RecordInvoicePayment(ctx context.Context, companyID, id string, amount float64, bankAccountID string) (*Invoice, error) {
	if bankAccountID == "" {
		return nil, ErrBankAccountRequired
	}
	if amount <= 0 {
		return nil, ErrInvalidStatus
	}
	return s.invoices.RecordPayment(ctx, companyID, id, amount, bankAccountID)
}

// ---- Expenses ----

func (s *Service) CreateExpense(ctx context.Context, e *Expense) error {
	return s.expenses.CreateExpense(ctx, e)
}

func (s *Service) GetExpense(ctx context.Context, companyID, id string) (*Expense, error) {
	return s.expenses.GetExpenseByID(ctx, companyID, id)
}

func (s *Service) ListExpenses(ctx context.Context, companyID string) ([]Expense, error) {
	return s.expenses.ListExpensesByCompany(ctx, companyID)
}

func (s *Service) UpdateExpense(ctx context.Context, e *Expense) error {
	return s.expenses.UpdateExpense(ctx, e)
}

func (s *Service) DeleteExpense(ctx context.Context, companyID, id string) error {
	return s.expenses.DeleteExpense(ctx, companyID, id)
}

func (s *Service) MarkExpensePaid(ctx context.Context, companyID, id, bankAccountID string) (*Expense, error) {
	if bankAccountID == "" {
		return nil, ErrBankAccountRequired
	}
	return s.expenses.MarkPaid(ctx, companyID, id, bankAccountID)
}

// ---- Alerts ----

func (s *Service) ListAlerts(ctx context.Context, companyID string, onlyUnresolved bool) ([]CashAlert, error) {
	return s.alerts.ListAlertsByCompany(ctx, companyID, onlyUnresolved)
}

func (s *Service) SetAlertResolved(ctx context.Context, companyID, id string, resolved bool) error {
	return s.alerts.SetAlertResolved(ctx, companyID, id, resolved)
}

// ---- Forecast ----

func (s *Service) GetForecast(ctx context.Context, companyID string, days int) ([]CashFlowProjection, error) {
	if days <= 0 || days > ForecastHorizonDays {
		days = 30
	}
	return s.projections.ListProjectionsByCompany(ctx, companyID, days)
}

// RecalculateForecast is the "batch job" logic: it walks the next
// ForecastHorizonDays days, projecting the balance forward from the sum of
// active bank accounts using unpaid invoices (by adjusted_due_date, which
// already carries the POD-driven delay from Module 4) and pending expenses
// (by due_date). It persists a snapshot per day and opens/updates a cash
// alert for any day the projection dips below the combined minimum balance.
func (s *Service) RecalculateForecast(ctx context.Context, companyID string) error {
	accounts, err := s.bankAccounts.ListBankAccountsByCompany(ctx, companyID)
	if err != nil {
		return err
	}

	// Single-currency assumption for this phase (agreed as MXN-only): a
	// company can hold accounts in other currencies (bank_accounts.currency),
	// but blending balances across currencies would produce a meaningless
	// sum, so only MXN accounts feed the forecast. Multi-currency projection
	// is out of scope here.
	var baseBalance, minThreshold float64
	for _, a := range accounts {
		if !a.IsActive || a.Currency != forecastCurrency {
			continue
		}
		baseBalance += a.CurrentBalance
		minThreshold += a.MinimumRequiredBalance
	}

	unpaidInvoices, err := s.invoices.ListUnpaidInvoicesByCompany(ctx, companyID)
	if err != nil {
		return err
	}
	pendingExpenses, err := s.expenses.ListPendingExpensesByCompany(ctx, companyID)
	if err != nil {
		return err
	}

	incomeByDate := map[string]float64{}
	for _, inv := range unpaidInvoices {
		remaining := inv.TotalAmount - inv.PaidAmount
		if remaining <= 0 {
			continue
		}
		incomeByDate[inv.AdjustedDueDate.Format(dateLayout)] += remaining
	}
	expenseByDate := map[string]float64{}
	for _, e := range pendingExpenses {
		expenseByDate[e.DueDate.Format(dateLayout)] += e.Amount
	}

	today := time.Now().Truncate(24 * time.Hour)
	rows := make([]CashFlowProjection, 0, ForecastHorizonDays)
	running := baseBalance

	for d := 1; d <= ForecastHorizonDays; d++ {
		date := today.AddDate(0, 0, d)
		key := date.Format(dateLayout)
		running += incomeByDate[key]
		running -= expenseByDate[key]

		rows = append(rows, CashFlowProjection{CompanyID: companyID, ProjectedDate: date, ProjectedBalance: running})

		if running < minThreshold {
			if err := s.raiseOrUpdateAlert(ctx, companyID, date, running, minThreshold); err != nil {
				return err
			}
		}
	}

	return s.projections.UpsertProjections(ctx, companyID, rows)
}

func (s *Service) raiseOrUpdateAlert(ctx context.Context, companyID string, date time.Time, projectedBalance, threshold float64) error {
	deficit := threshold - projectedBalance
	severity := severityFor(projectedBalance, threshold)
	description := fmt.Sprintf("Saldo proyectado por debajo del mínimo requerido el %s (déficit estimado %.2f)",
		date.Format(dateLayout), deficit)

	existing, err := s.alerts.GetOpenAlertByCompanyAndDate(ctx, companyID, date)
	if err != nil {
		return err
	}
	if existing != nil {
		existing.Severity = severity
		existing.ProjectedDeficit = deficit
		existing.Description = description
		return s.alerts.UpdateAlert(ctx, existing)
	}

	return s.alerts.CreateAlert(ctx, &CashAlert{
		CompanyID:        companyID,
		ProjectedDate:    date,
		Severity:         severity,
		ProjectedDeficit: deficit,
		Description:      description,
	})
}

func severityFor(balance, threshold float64) string {
	if balance < 0 {
		return "critical"
	}
	if threshold <= 0 {
		return "low"
	}
	ratio := (threshold - balance) / threshold
	switch {
	case ratio > 0.5:
		return "high"
	case ratio > 0.2:
		return "medium"
	default:
		return "low"
	}
}
