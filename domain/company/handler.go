package company

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterPlatformRoutes is mounted under /platform, guarded by the
// platform_admin role only — client companies no longer self-signup.
func (h *Handler) RegisterPlatformRoutes(router fiber.Router) {
	router.Post("/companies", h.Signup)
	router.Get("/companies", h.ListCompanies)
	router.Patch("/companies/:id", h.SetCompanyActive)
}

// RegisterRoutes is mounted under a protected /company group. writeGuard
// restricts create/update/delete to admin+operations; reads are open to any
// authenticated user of the company (admin, operations, finance).
func (h *Handler) RegisterRoutes(router fiber.Router, writeGuard, adminOnly fiber.Handler) {
	router.Post("/teammates", adminOnly, h.InviteTeammate)

	router.Get("/vehicles", h.ListVehicles)
	router.Post("/vehicles", writeGuard, h.CreateVehicle)
	router.Put("/vehicles/:id", writeGuard, h.UpdateVehicle)
	router.Delete("/vehicles/:id", writeGuard, h.DeleteVehicle)

	router.Get("/routes", h.ListRoutes)
	router.Post("/routes", writeGuard, h.CreateRoute)
	router.Put("/routes/:id", writeGuard, h.UpdateRoute)
	router.Delete("/routes/:id", writeGuard, h.DeleteRoute)

	router.Get("/clients", h.ListClients)
	router.Post("/clients", writeGuard, h.CreateClient)
	router.Put("/clients/:id", writeGuard, h.UpdateClient)
	router.Delete("/clients/:id", writeGuard, h.DeleteClient)

	router.Get("/contracts", h.ListContracts)
	router.Post("/contracts", writeGuard, h.CreateContract)
	router.Put("/contracts/:id", writeGuard, h.UpdateContract)
	router.Delete("/contracts/:id", writeGuard, h.DeleteContract)
}

func companyIDFromCtx(c *fiber.Ctx) string {
	id, _ := c.Locals("company_id").(string)
	return id
}

func handleCompanyError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	case errors.Is(err, ErrInvalidRole):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, auth.ErrEmailAlreadyExists):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
}

// ---- Signup ----

type signupRequest struct {
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (h *Handler) Signup(c *fiber.Ctx) error {
	var req signupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.CompanyName == "" || req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "company_name, email and password are required"})
	}

	result, err := h.svc.Signup(c.Context(), req.CompanyName, req.Email, req.Password)
	if err != nil {
		return handleCompanyError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"company_id": result.Company.ID,
		"user_id":    result.User.ID,
		"email":      result.User.Email,
	})
}

func (h *Handler) ListCompanies(c *fiber.Ctx) error {
	list, err := h.svc.ListCompanies(c.Context())
	if err != nil {
		return handleCompanyError(c, err)
	}
	return c.JSON(list)
}

type setCompanyActiveRequest struct {
	IsActive bool `json:"is_active"`
}

func (h *Handler) SetCompanyActive(c *fiber.Ctx) error {
	var req setCompanyActiveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.svc.SetCompanyActive(c.Context(), c.Params("id"), req.IsActive); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Teammates ----

type inviteTeammateRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) InviteTeammate(c *fiber.Ctx) error {
	var req inviteTeammateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Email == "" || req.Password == "" || req.Role == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email, password and role are required"})
	}

	u, err := h.svc.InviteTeammate(c.Context(), companyIDFromCtx(c), req.Email, req.Password, req.Role)
	if err != nil {
		return handleCompanyError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": u.ID, "email": u.Email, "role": req.Role})
}

// ---- Vehicles ----

type vehicleRequest struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
	IsActive   *bool  `json:"is_active"`
}

func (h *Handler) CreateVehicle(c *fiber.Ctx) error {
	var req vehicleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	v := &Vehicle{CompanyID: companyIDFromCtx(c), Type: req.Type, Identifier: req.Identifier}
	if err := h.svc.CreateVehicle(c.Context(), v); err != nil {
		return handleCompanyError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(v)
}

func (h *Handler) ListVehicles(c *fiber.Ctx) error {
	list, err := h.svc.ListVehicles(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleCompanyError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateVehicle(c *fiber.Ctx) error {
	var req vehicleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	v := &Vehicle{ID: c.Params("id"), CompanyID: companyIDFromCtx(c), Type: req.Type, Identifier: req.Identifier, IsActive: isActive}
	if err := h.svc.UpdateVehicle(c.Context(), v); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteVehicle(c *fiber.Ctx) error {
	if err := h.svc.DeleteVehicle(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Routes ----

type routeRequest struct {
	Origin      string   `json:"origin"`
	Destination string   `json:"destination"`
	DistanceKM  *float64 `json:"distance_km"`
	IsActive    *bool    `json:"is_active"`
}

func (h *Handler) CreateRoute(c *fiber.Ctx) error {
	var req routeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	r := &Route{CompanyID: companyIDFromCtx(c), Origin: req.Origin, Destination: req.Destination, DistanceKM: req.DistanceKM}
	if err := h.svc.CreateRoute(c.Context(), r); err != nil {
		return handleCompanyError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(r)
}

func (h *Handler) ListRoutes(c *fiber.Ctx) error {
	list, err := h.svc.ListRoutes(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleCompanyError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateRoute(c *fiber.Ctx) error {
	var req routeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	r := &Route{ID: c.Params("id"), CompanyID: companyIDFromCtx(c), Origin: req.Origin, Destination: req.Destination, DistanceKM: req.DistanceKM, IsActive: isActive}
	if err := h.svc.UpdateRoute(c.Context(), r); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteRoute(c *fiber.Ctx) error {
	if err := h.svc.DeleteRoute(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Clients ----

type clientRequest struct {
	Name     string  `json:"name"`
	TaxID    *string `json:"tax_id"`
	IsActive *bool   `json:"is_active"`
}

func (h *Handler) CreateClient(c *fiber.Ctx) error {
	var req clientRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	cl := &Client{CompanyID: companyIDFromCtx(c), Name: req.Name, TaxID: req.TaxID}
	if err := h.svc.CreateClient(c.Context(), cl); err != nil {
		return handleCompanyError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(cl)
}

func (h *Handler) ListClients(c *fiber.Ctx) error {
	list, err := h.svc.ListClients(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleCompanyError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateClient(c *fiber.Ctx) error {
	var req clientRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	cl := &Client{ID: c.Params("id"), CompanyID: companyIDFromCtx(c), Name: req.Name, TaxID: req.TaxID, IsActive: isActive}
	if err := h.svc.UpdateClient(c.Context(), cl); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteClient(c *fiber.Ctx) error {
	if err := h.svc.DeleteClient(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Contracts ----

type contractRequest struct {
	ClientID  string  `json:"client_id"`
	Reference string  `json:"reference"`
	StartsOn  *string `json:"starts_on"`
	EndsOn    *string `json:"ends_on"`
	IsActive  *bool   `json:"is_active"`
}

func (h *Handler) CreateContract(c *fiber.Ctx) error {
	var req contractRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	startsOn, endsOn, err := parseContractDates(req.StartsOn, req.EndsOn)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	ct := &Contract{CompanyID: companyIDFromCtx(c), ClientID: req.ClientID, Reference: req.Reference, StartsOn: startsOn, EndsOn: endsOn}
	if err := h.svc.CreateContract(c.Context(), ct); err != nil {
		return handleCompanyError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(ct)
}

func (h *Handler) ListContracts(c *fiber.Ctx) error {
	list, err := h.svc.ListContracts(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleCompanyError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateContract(c *fiber.Ctx) error {
	var req contractRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	startsOn, endsOn, err := parseContractDates(req.StartsOn, req.EndsOn)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	ct := &Contract{
		ID: c.Params("id"), CompanyID: companyIDFromCtx(c), ClientID: req.ClientID,
		Reference: req.Reference, StartsOn: startsOn, EndsOn: endsOn, IsActive: isActive,
	}
	if err := h.svc.UpdateContract(c.Context(), ct); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteContract(c *fiber.Ctx) error {
	if err := h.svc.DeleteContract(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleCompanyError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
