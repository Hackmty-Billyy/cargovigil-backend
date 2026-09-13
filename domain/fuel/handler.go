package fuel

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

// RegisterPlatformRoutes is mounted under the existing /platform group
// (platform_admin only, see server.go): fuel_indexes has no company_id, it's
// a shared reference feed across every tenant, so only CargoVigil's own
// staff can write to it — same reasoning as company onboarding.
func (h *Handler) RegisterPlatformRoutes(router fiber.Router) {
	router.Post("/fuel-indexes", h.RecordFuelIndex)
}

// RegisterRoutes is mounted under a protected /fuel group. Reads of
// fuel_indexes and trip_fuel_logs are open to any authenticated role of the
// company (like the catalog); logging an actual purchase is operational
// data entry (opsWriteGuard: admin+operations, same as trip_frictions would
// be). Surcharge rules and margin simulations are pricing/financial
// decisions, gated financeGuard (admin+finance), matching treasury.
func (h *Handler) RegisterRoutes(router fiber.Router, opsWriteGuard, financeGuard fiber.Handler) {
	router.Get("/indexes", h.ListFuelIndexes)
	router.Get("/indexes/weekly-trend", h.ListFuelPriceWeeklyTrend)

	router.Get("/trip-logs", h.ListTripFuelLogs)
	router.Get("/trip-logs/trip/:tripId", h.ListTripFuelLogsForTrip)
	router.Post("/trip-logs", opsWriteGuard, h.CreateTripFuelLog)
	router.Delete("/trip-logs/:id", opsWriteGuard, h.DeleteTripFuelLog)

	router.Get("/surcharge-rules", financeGuard, h.ListSurchargeRules)
	router.Post("/surcharge-rules", financeGuard, h.CreateSurchargeRule)
	router.Put("/surcharge-rules/:id", financeGuard, h.UpdateSurchargeRule)
	router.Delete("/surcharge-rules/:id", financeGuard, h.DeleteSurchargeRule)
	router.Post("/surcharge-rules/recalculate", financeGuard, h.RecalculateSurchargeRules)

	router.Get("/margin-impacts", financeGuard, h.ListRouteMarginImpacts)
	router.Get("/routes/:routeId/margin-impact", financeGuard, h.GetRouteMarginImpact)
	router.Post("/routes/:routeId/margin-impact/simulate", financeGuard, h.SimulateRouteMarginImpact)
}

func companyIDFromCtx(c *fiber.Ctx) string {
	id, _ := c.Locals("company_id").(string)
	return id
}

func handleFuelError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	case errors.Is(err, ErrDuplicateRule):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrInvalidFuelType), errors.Is(err, ErrInvalidVariation):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrInsufficientData):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
}

// ---- Fuel indexes ----

type fuelIndexRequest struct {
	FuelType      string  `json:"fuel_type"`
	Region        string  `json:"region"`
	PricePerUnit  float64 `json:"price_per_unit"`
	UnitOfMeasure string  `json:"unit_of_measure"`
	Source        string  `json:"source"`
}

func (h *Handler) RecordFuelIndex(c *fiber.Ctx) error {
	var req fuelIndexRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	idx := &FuelIndex{
		FuelType: req.FuelType, Region: req.Region, PricePerUnit: req.PricePerUnit,
		UnitOfMeasure: req.UnitOfMeasure, Source: req.Source,
	}
	if err := h.svc.RecordFuelIndex(c.Context(), idx); err != nil {
		return handleFuelError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(idx)
}

func (h *Handler) ListFuelPriceWeeklyTrend(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "12"))
	list, err := h.svc.ListFuelPriceWeeklyTrend(c.Context(), c.Query("fuel_type"), c.Query("region"), limit)
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) ListFuelIndexes(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	list, err := h.svc.ListFuelIndexes(c.Context(), c.Query("fuel_type"), c.Query("region"), limit)
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(list)
}

// ---- Trip fuel logs ----

type tripFuelLogRequest struct {
	TripID          string   `json:"trip_id"`
	FuelType        string   `json:"fuel_type"`
	VolumePurchased float64  `json:"volume_purchased"`
	CostPerUnit     float64  `json:"cost_per_unit"`
	TotalCost       float64  `json:"total_cost"`
	OdometerOrHours *float64 `json:"odometer_or_hours"`
	PurchasedAt     *string  `json:"purchased_at"`
}

func (h *Handler) CreateTripFuelLog(c *fiber.Ctx) error {
	var req tripFuelLogRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	l := &TripFuelLog{
		CompanyID: companyIDFromCtx(c), TripID: req.TripID, FuelType: req.FuelType,
		VolumePurchased: req.VolumePurchased, CostPerUnit: req.CostPerUnit, TotalCost: req.TotalCost,
		OdometerOrHours: req.OdometerOrHours,
	}
	if req.PurchasedAt != nil && *req.PurchasedAt != "" {
		t, err := parseTimestamp(*req.PurchasedAt)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid purchased_at"})
		}
		l.PurchasedAt = t
	}
	if err := h.svc.CreateTripFuelLog(c.Context(), l); err != nil {
		return handleFuelError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(l)
}

func (h *Handler) ListTripFuelLogs(c *fiber.Ctx) error {
	list, err := h.svc.ListTripFuelLogs(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) ListTripFuelLogsForTrip(c *fiber.Ctx) error {
	list, err := h.svc.ListTripFuelLogsForTrip(c.Context(), companyIDFromCtx(c), c.Params("tripId"))
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) DeleteTripFuelLog(c *fiber.Ctx) error {
	if err := h.svc.DeleteTripFuelLog(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleFuelError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Fuel surcharge rules ----

type surchargeRuleRequest struct {
	FuelType            string  `json:"fuel_type"`
	Region              string  `json:"region"`
	BaselinePrice       float64 `json:"baseline_price"`
	ThresholdPercentage float64 `json:"threshold_percentage"`
	PassThroughRate     float64 `json:"pass_through_rate"`
	IsActive            *bool   `json:"is_active"`
}

func (h *Handler) CreateSurchargeRule(c *fiber.Ctx) error {
	var req surchargeRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	r := &FuelSurchargeRule{
		CompanyID: companyIDFromCtx(c), FuelType: req.FuelType, Region: req.Region,
		BaselinePrice: req.BaselinePrice, ThresholdPercentage: req.ThresholdPercentage, PassThroughRate: req.PassThroughRate,
	}
	if err := h.svc.CreateSurchargeRule(c.Context(), r); err != nil {
		return handleFuelError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(r)
}

func (h *Handler) ListSurchargeRules(c *fiber.Ctx) error {
	list, err := h.svc.ListSurchargeRules(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) UpdateSurchargeRule(c *fiber.Ctx) error {
	var req surchargeRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	r := &FuelSurchargeRule{
		ID: c.Params("id"), CompanyID: companyIDFromCtx(c),
		BaselinePrice: req.BaselinePrice, ThresholdPercentage: req.ThresholdPercentage, PassThroughRate: req.PassThroughRate,
		IsActive: isActive,
	}
	if err := h.svc.UpdateSurchargeRule(c.Context(), r); err != nil {
		return handleFuelError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteSurchargeRule(c *fiber.Ctx) error {
	if err := h.svc.DeleteSurchargeRule(c.Context(), companyIDFromCtx(c), c.Params("id")); err != nil {
		return handleFuelError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) RecalculateSurchargeRules(c *fiber.Ctx) error {
	if err := h.svc.RecalculateSurchargeRules(c.Context(), companyIDFromCtx(c)); err != nil {
		return handleFuelError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Route margin impact ----

func (h *Handler) ListRouteMarginImpacts(c *fiber.Ctx) error {
	list, err := h.svc.ListRouteMarginImpacts(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) GetRouteMarginImpact(c *fiber.Ctx) error {
	fuelType := c.Query("fuel_type", "diesel")
	impact, err := h.svc.GetRouteMarginImpact(c.Context(), companyIDFromCtx(c), c.Params("routeId"), fuelType)
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(impact)
}

type simulateMarginImpactRequest struct {
	FuelType                 string  `json:"fuel_type"`
	PriceVariationPercentage float64 `json:"price_variation_percentage"`
}

func (h *Handler) SimulateRouteMarginImpact(c *fiber.Ctx) error {
	var req simulateMarginImpactRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.FuelType == "" {
		req.FuelType = "diesel"
	}
	impact, err := h.svc.SimulateRouteMarginImpact(c.Context(), companyIDFromCtx(c), c.Params("routeId"), req.FuelType, req.PriceVariationPercentage)
	if err != nil {
		return handleFuelError(c, err)
	}
	return c.JSON(impact)
}
