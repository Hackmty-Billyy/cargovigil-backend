package routecost

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the module. The group itself is already authenticated
// at the server level; the guards are applied per route because this module
// mixes two sensitivities: trips and frictions are operational data (any role
// of the company reads them, admin+operations write them), while the
// contingency cushion moves reserved money and is admin+finance to write.
//
// Per-route middleware is used instead of nested groups on purpose: a
// router.Group("", guard) on this same router would also leak the guard onto
// routes registered after it.
func (h *Handler) RegisterRoutes(router fiber.Router, writeGuard, financeGuard fiber.Handler) {
	router.Get("/trips", h.ListTrips)
	router.Post("/trips", writeGuard, h.CreateTrip)
	router.Get("/trips/:id", h.GetTrip)
	router.Put("/trips/:id", writeGuard, h.UpdateTrip)
	router.Delete("/trips/:id", writeGuard, h.DeleteTrip)
	router.Patch("/trips/:id/status", writeGuard, h.ChangeTripStatus)

	router.Get("/trips/:id/frictions", h.ListTripFrictions)
	router.Post("/trips/:id/frictions", writeGuard, h.RegisterFriction)
	router.Get("/trips/:id/impact", h.GetTripImpact)

	router.Get("/frictions", h.ListFrictions)
	router.Patch("/frictions/:id/close", writeGuard, h.CloseFriction)

	router.Get("/risk-profiles", h.ListRiskProfiles)
	router.Post("/risk-profiles/recalculate", writeGuard, h.RecalculateRiskProfiles)

	router.Get("/contingency", h.ListContingencyFunds)
	router.Get("/trips/:id/contingency", h.GetContingency)
	router.Post("/trips/:id/contingency/allocate", financeGuard, h.AllocateContingency)
	router.Post("/trips/:id/contingency/release", financeGuard, h.ReleaseContingency)

	router.Get("/settings", h.GetCostSettings)
	router.Put("/settings", financeGuard, h.UpdateCostSettings)
}

func companyIDFromCtx(c *fiber.Ctx) string {
	id, _ := c.Locals("company_id").(string)
	return id
}

func handleRouteCostError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	case errors.Is(err, ErrInvalidReference), errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrInvalidTransition),
		errors.Is(err, ErrInvalidEventType), errors.Is(err, ErrInvalidDates), errors.Is(err, ErrFrictionClosed),
		errors.Is(err, ErrFundReleased), errors.Is(err, ErrUnsupportedCurrency), errors.Is(err, ErrInvalidRate),
		errors.Is(err, ErrDuplicateTracking):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
}

// ---- Trips ----

type tripRequest struct {
	VehicleID            string  `json:"vehicle_id"`
	RouteID              string  `json:"route_id"`
	ClientID             string  `json:"client_id"`
	ContractID           *string `json:"contract_id"`
	TrackingCode         string  `json:"tracking_code"`
	CargoType            string  `json:"cargo_type"`
	CargoWeightTons      float64 `json:"cargo_weight_tons"`
	Status               string  `json:"status"`
	DepartureDate        string  `json:"departure_date"`
	EstimatedArrivalDate string  `json:"estimated_arrival_date"`
	ActualArrivalDate    *string `json:"actual_arrival_date"`
	AgreedFreightPrice   float64 `json:"agreed_freight_price"`
	Currency             string  `json:"currency"`
	FuelSurchargeAmount  float64 `json:"fuel_surcharge_amount"`
}

func (r tripRequest) toTrip(companyID string) (*Trip, error) {
	departure, err := parseTimestamp(r.DepartureDate)
	if err != nil {
		return nil, ErrInvalidDates
	}
	estimated, err := parseTimestamp(r.EstimatedArrivalDate)
	if err != nil {
		return nil, ErrInvalidDates
	}
	actual, err := parseOptionalTimestamp(r.ActualArrivalDate)
	if err != nil {
		return nil, ErrInvalidDates
	}
	cargoType := r.CargoType
	if cargoType == "" {
		cargoType = "general"
	}
	return &Trip{
		CompanyID: companyID, VehicleID: r.VehicleID, RouteID: r.RouteID, ClientID: r.ClientID,
		ContractID: r.ContractID, TrackingCode: r.TrackingCode, CargoType: cargoType,
		CargoWeightTons: r.CargoWeightTons, Status: r.Status, DepartureDate: departure,
		EstimatedArrivalDate: estimated, ActualArrivalDate: actual, AgreedFreightPrice: r.AgreedFreightPrice,
		Currency: r.Currency, FuelSurchargeAmount: r.FuelSurchargeAmount,
	}, nil
}

func (h *Handler) CreateTrip(c *fiber.Ctx) error {
	var req tripRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	trip, err := req.toTrip(companyIDFromCtx(c))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	created, err := h.svc.CreateTrip(c.Context(), trip)
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *Handler) ListTrips(c *fiber.Ctx) error {
	filter := TripFilter{Status: c.Query("status"), RouteID: c.Query("route_id")}
	list, err := h.svc.ListTrips(c.Context(), companyIDFromCtx(c), filter)
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) GetTrip(c *fiber.Ctx) error {
	trip, err := h.svc.GetTrip(c.Context(), companyIDFromCtx(c), c.Params("id"))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(trip)
}

func (h *Handler) UpdateTrip(c *fiber.Ctx) error {
	var req tripRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	trip, err := req.toTrip(companyIDFromCtx(c))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	trip.ID = c.Params("id")
	if err := h.svc.UpdateTrip(c.Context(), trip); err != nil {
		return handleRouteCostError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteTrip(c *fiber.Ctx) error {
	if err := h.svc.DeleteTrip(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleRouteCostError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type tripStatusRequest struct {
	Status            string  `json:"status"`
	ActualArrivalDate *string `json:"actual_arrival_date"`
}

func (h *Handler) ChangeTripStatus(c *fiber.Ctx) error {
	var req tripStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	actual, err := parseOptionalTimestamp(req.ActualArrivalDate)
	if err != nil {
		return handleRouteCostError(c, ErrInvalidDates)
	}
	trip, err := h.svc.ChangeTripStatus(c.Context(), companyIDFromCtx(c), c.Params("id"), req.Status, actual)
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(trip)
}

// ---- Frictions ----

type frictionRequest struct {
	EventType    string  `json:"event_type"`
	LocationName *string `json:"location_name"`
	StartedAt    string  `json:"started_at"`
	EndedAt      *string `json:"ended_at"`
	CostImpact   float64 `json:"cost_impact"`
	Notes        *string `json:"notes"`
}

func (h *Handler) RegisterFriction(c *fiber.Ctx) error {
	var req frictionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	startedAt, err := parseTimestamp(req.StartedAt)
	if err != nil {
		return handleRouteCostError(c, ErrInvalidDates)
	}
	endedAt, err := parseOptionalTimestamp(req.EndedAt)
	if err != nil {
		return handleRouteCostError(c, ErrInvalidDates)
	}

	f, err := h.svc.RegisterFriction(c.Context(), companyIDFromCtx(c), FrictionInput{
		TripID: c.Params("id"), EventType: req.EventType, LocationName: req.LocationName,
		StartedAt: startedAt, EndedAt: endedAt, CostImpact: req.CostImpact, Notes: req.Notes,
	})
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(f)
}

func (h *Handler) ListTripFrictions(c *fiber.Ctx) error {
	list, err := h.svc.ListFrictionsByTrip(c.Context(), companyIDFromCtx(c), c.Params("id"))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) ListFrictions(c *fiber.Ctx) error {
	list, err := h.svc.ListFrictions(c.Context(), companyIDFromCtx(c), c.Query("open") == "true")
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(list)
}

type closeFrictionRequest struct {
	EndedAt    string   `json:"ended_at"`
	CostImpact *float64 `json:"cost_impact"`
	Notes      *string  `json:"notes"`
}

func (h *Handler) CloseFriction(c *fiber.Ctx) error {
	var req closeFrictionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	endedAt, err := parseTimestamp(req.EndedAt)
	if err != nil {
		return handleRouteCostError(c, ErrInvalidDates)
	}
	f, err := h.svc.CloseFriction(c.Context(), companyIDFromCtx(c), c.Params("id"), endedAt, req.CostImpact, req.Notes)
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(f)
}

func (h *Handler) GetTripImpact(c *fiber.Ctx) error {
	impact, err := h.svc.TripImpact(c.Context(), companyIDFromCtx(c), c.Params("id"))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(impact)
}

// ---- Route risk ----

func (h *Handler) ListRiskProfiles(c *fiber.Ctx) error {
	list, err := h.svc.ListRiskProfiles(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) RecalculateRiskProfiles(c *fiber.Ctx) error {
	companyID := companyIDFromCtx(c)
	if routeID := c.Query("route_id"); routeID != "" {
		profile, err := h.svc.RecalculateRouteRisk(c.Context(), companyID, routeID)
		if err != nil {
			return handleRouteCostError(c, err)
		}
		return c.JSON([]RouteRiskProfile{*profile})
	}
	profiles, err := h.svc.RecalculateAllRouteRisks(c.Context(), companyID)
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(profiles)
}

// ---- Contingency ----

func (h *Handler) ListContingencyFunds(c *fiber.Ctx) error {
	list, err := h.svc.ListContingencyFunds(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) GetContingency(c *fiber.Ctx) error {
	fund, err := h.svc.GetContingency(c.Context(), companyIDFromCtx(c), c.Params("id"))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(fund)
}

func (h *Handler) AllocateContingency(c *fiber.Ctx) error {
	fund, err := h.svc.AllocateContingency(c.Context(), companyIDFromCtx(c), c.Params("id"))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(fund)
}

func (h *Handler) ReleaseContingency(c *fiber.Ctx) error {
	fund, err := h.svc.ReleaseContingency(c.Context(), companyIDFromCtx(c), c.Params("id"))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(fund)
}

// ---- Cost settings ----

type costSettingsRequest struct {
	IdleHourlyRate *float64 `json:"idle_hourly_rate"`
	Currency       string   `json:"currency"`
}

func (h *Handler) GetCostSettings(c *fiber.Ctx) error {
	settings, err := h.svc.GetCostSettings(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(settings)
}

func (h *Handler) UpdateCostSettings(c *fiber.Ctx) error {
	var req costSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	settings := &CostSettings{CompanyID: companyIDFromCtx(c), IdleHourlyRate: req.IdleHourlyRate, Currency: req.Currency}
	if err := h.svc.UpdateCostSettings(c.Context(), settings); err != nil {
		return handleRouteCostError(c, err)
	}
	return c.JSON(settings)
}
