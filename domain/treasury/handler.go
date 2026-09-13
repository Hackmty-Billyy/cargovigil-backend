package treasury

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts every treasury endpoint. The whole group is gated by
// admin+finance at the server level — unlike the company catalog, operations
// has no read access here either.
func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Get("/bank-accounts", h.ListBankAccounts)
	router.Post("/bank-accounts", h.CreateBankAccount)
	router.Put("/bank-accounts/:id", h.UpdateBankAccount)
	router.Delete("/bank-accounts/:id", h.DeleteBankAccount)

	router.Get("/invoices", h.ListInvoices)
	router.Post("/invoices", h.CreateInvoice)
	router.Put("/invoices/:id", h.UpdateInvoice)
	router.Delete("/invoices/:id", h.DeleteInvoice)
	router.Post("/invoices/:id/payments", h.RecordInvoicePayment)

	router.Get("/expenses", h.ListExpenses)
	router.Post("/expenses", h.CreateExpense)
	router.Put("/expenses/:id", h.UpdateExpense)
	router.Delete("/expenses/:id", h.DeleteExpense)
	router.Post("/expenses/:id/pay", h.MarkExpensePaid)

	router.Get("/forecast", h.GetForecast)
	router.Post("/forecast/recalculate", h.RecalculateForecast)

	router.Get("/alerts", h.ListAlerts)
	router.Patch("/alerts/:id", h.SetAlertResolved)
}

func companyIDFromCtx(c *fiber.Ctx) string {
	id, _ := c.Locals("company_id").(string)
	return id
}

func handleTreasuryError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	case errors.Is(err, ErrBankAccountRequired), errors.Is(err, ErrOverpayment), errors.Is(err, ErrInvalidStatus):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
}

// ---- Bank accounts ----

type bankAccountRequest struct {
	BankName               string  `json:"bank_name"`
	AccountNumberMask      string  `json:"account_number_mask"`
	Currency               string  `json:"currency"`
	CurrentBalance         float64 `json:"current_balance"`
	MinimumRequiredBalance float64 `json:"minimum_required_balance"`
	IsActive               *bool   `json:"is_active"`
}

func (h *Handler) CreateBankAccount(c *fiber.Ctx) error {
	var req bankAccountRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	a := &BankAccount{
		CompanyID:              companyIDFromCtx(c),
		BankName:               req.BankName,
		AccountNumberMask:      req.AccountNumberMask,
		Currency:               req.Currency,
		CurrentBalance:         req.CurrentBalance,
		MinimumRequiredBalance: req.MinimumRequiredBalance,
	}
	if err := h.svc.CreateBankAccount(c.Context(), a); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(a)
}

func (h *Handler) ListBankAccounts(c *fiber.Ctx) error {
	list, err := h.svc.ListBankAccounts(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleTreasuryError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateBankAccount(c *fiber.Ctx) error {
	var req bankAccountRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	a := &BankAccount{
		ID: c.Params("id"), CompanyID: companyIDFromCtx(c),
		BankName: req.BankName, AccountNumberMask: req.AccountNumberMask, Currency: req.Currency,
		MinimumRequiredBalance: req.MinimumRequiredBalance, IsActive: isActive,
	}
	if err := h.svc.UpdateBankAccount(c.Context(), a); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteBankAccount(c *fiber.Ctx) error {
	if err := h.svc.DeleteBankAccount(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Invoices ----

type invoiceRequest struct {
	TripID          *string `json:"trip_id"`
	ClientID        string  `json:"client_id"`
	InvoiceNumber   string  `json:"invoice_number"`
	IssueDate       string  `json:"issue_date"`
	DueDate         string  `json:"due_date"`
	AdjustedDueDate *string `json:"adjusted_due_date"`
	TotalAmount     float64 `json:"total_amount"`
}

func (h *Handler) CreateInvoice(c *fiber.Ctx) error {
	var req invoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	issueDate, err := parseDate(req.IssueDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid issue_date"})
	}
	dueDate, err := parseDate(req.DueDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid due_date"})
	}
	adjustedDueDate := dueDate
	if req.AdjustedDueDate != nil && *req.AdjustedDueDate != "" {
		adjustedDueDate, err = parseDate(*req.AdjustedDueDate)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid adjusted_due_date"})
		}
	}

	inv := &Invoice{
		CompanyID: companyIDFromCtx(c), TripID: req.TripID, ClientID: req.ClientID,
		InvoiceNumber: req.InvoiceNumber, IssueDate: issueDate, DueDate: dueDate,
		AdjustedDueDate: adjustedDueDate, TotalAmount: req.TotalAmount,
	}
	if err := h.svc.CreateInvoice(c.Context(), inv); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(inv)
}

func (h *Handler) ListInvoices(c *fiber.Ctx) error {
	list, err := h.svc.ListInvoices(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleTreasuryError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateInvoice(c *fiber.Ctx) error {
	var req invoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	issueDate, err := parseDate(req.IssueDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid issue_date"})
	}
	dueDate, err := parseDate(req.DueDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid due_date"})
	}
	adjustedDueDate := dueDate
	if req.AdjustedDueDate != nil && *req.AdjustedDueDate != "" {
		adjustedDueDate, err = parseDate(*req.AdjustedDueDate)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid adjusted_due_date"})
		}
	}

	inv := &Invoice{
		ID: c.Params("id"), CompanyID: companyIDFromCtx(c), TripID: req.TripID, ClientID: req.ClientID,
		InvoiceNumber: req.InvoiceNumber, IssueDate: issueDate, DueDate: dueDate,
		AdjustedDueDate: adjustedDueDate, TotalAmount: req.TotalAmount,
	}
	if err := h.svc.UpdateInvoice(c.Context(), inv); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteInvoice(c *fiber.Ctx) error {
	if err := h.svc.DeleteInvoice(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type paymentRequest struct {
	Amount        float64 `json:"amount"`
	BankAccountID string  `json:"bank_account_id"`
}

func (h *Handler) RecordInvoicePayment(c *fiber.Ctx) error {
	var req paymentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	inv, err := h.svc.RecordInvoicePayment(c.Context(), companyIDFromCtx(c), c.Params("id"), req.Amount, req.BankAccountID)
	if err != nil {
		return handleTreasuryError(c, err)
	}
	return c.JSON(inv)
}

// ---- Expenses ----

type expenseRequest struct {
	TripID      *string `json:"trip_id"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	DueDate     string  `json:"due_date"`
}

func (h *Handler) CreateExpense(c *fiber.Ctx) error {
	var req expenseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	dueDate, err := parseDate(req.DueDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid due_date"})
	}
	e := &Expense{
		CompanyID: companyIDFromCtx(c), TripID: req.TripID, Category: req.Category,
		Description: req.Description, Amount: req.Amount, DueDate: dueDate,
	}
	if err := h.svc.CreateExpense(c.Context(), e); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(e)
}

func (h *Handler) ListExpenses(c *fiber.Ctx) error {
	list, err := h.svc.ListExpenses(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleTreasuryError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateExpense(c *fiber.Ctx) error {
	var req expenseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	dueDate, err := parseDate(req.DueDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid due_date"})
	}
	e := &Expense{
		ID: c.Params("id"), CompanyID: companyIDFromCtx(c), TripID: req.TripID, Category: req.Category,
		Description: req.Description, Amount: req.Amount, DueDate: dueDate,
	}
	if err := h.svc.UpdateExpense(c.Context(), e); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteExpense(c *fiber.Ctx) error {
	if err := h.svc.DeleteExpense(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type markPaidRequest struct {
	BankAccountID string `json:"bank_account_id"`
}

func (h *Handler) MarkExpensePaid(c *fiber.Ctx) error {
	var req markPaidRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	e, err := h.svc.MarkExpensePaid(c.Context(), companyIDFromCtx(c), c.Params("id"), req.BankAccountID)
	if err != nil {
		return handleTreasuryError(c, err)
	}
	return c.JSON(e)
}

// ---- Forecast & alerts ----

func (h *Handler) GetForecast(c *fiber.Ctx) error {
	days, _ := strconv.Atoi(c.Query("days", "30"))
	rows, err := h.svc.GetForecast(c.Context(), companyIDFromCtx(c), days)
	if err != nil {
		return handleTreasuryError(c, err)
	}
	return c.JSON(rows)
}

func (h *Handler) RecalculateForecast(c *fiber.Ctx) error {
	if err := h.svc.RecalculateForecast(c.Context(), companyIDFromCtx(c)); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) ListAlerts(c *fiber.Ctx) error {
	onlyUnresolved := c.Query("resolved") != "true"
	list, err := h.svc.ListAlerts(c.Context(), companyIDFromCtx(c), onlyUnresolved)
	if err != nil {
		return handleTreasuryError(c, err)
	}
	return c.JSON(list)
}

type setAlertResolvedRequest struct {
	IsResolved bool `json:"is_resolved"`
}

func (h *Handler) SetAlertResolved(c *fiber.Ctx) error {
	var req setAlertResolvedRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.svc.SetAlertResolved(c.Context(), companyIDFromCtx(c), c.Params("id"), req.IsResolved); err != nil {
		return handleTreasuryError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
