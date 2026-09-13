package logistics

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/routecost"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the radar endpoints. Reads are open to every role of
// the company; creating a trip goes through the same admin+operations guard as
// /routecost, because it commits money. Advancing the simulation is left open
// to the three roles on purpose: it is a demo/telemetry stand-in, not a
// financial operation, and the radar shows that button to everyone.
func (h *Handler) RegisterRoutes(router fiber.Router, writeGuard fiber.Handler) {
	router.Get("/trips", h.ListTrips)
	router.Post("/trips/projection", h.Project)
	router.Post("/trips/advance-simulation", h.AdvanceSimulation)
	router.Post("/trips", writeGuard, h.CreateTrip)
	router.Get("/trips/:id", h.GetTrip)
	router.Get("/fuel-indexes", h.ListFuelIndexes)
}

func companyIDFromCtx(c *fiber.Ctx) string {
	id, _ := c.Locals("company_id").(string)
	return id
}

func handleLogisticsError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, routecost.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	case errors.Is(err, ErrInvalidVehicle), errors.Is(err, ErrInvalidDates), errors.Is(err, ErrNoFuelIndex),
		errors.Is(err, ErrInvalidCurrency), errors.Is(err, routecost.ErrInvalidReference),
		errors.Is(err, routecost.ErrDuplicateTracking), errors.Is(err, routecost.ErrInvalidDates),
		errors.Is(err, routecost.ErrInvalidStatus), errors.Is(err, routecost.ErrUnsupportedCurrency):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
}

// timestampLayouts covers what the dashboard actually sends: an <input
// type="datetime-local"> has no timezone and no seconds, so a naive value is
// read as UTC rather than rejected.
var timestampLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

func parseTimestamp(raw string) (time.Time, error) {
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, ErrInvalidDates
}

func (h *Handler) ListTrips(c *fiber.Ctx) error {
	trips, err := h.svc.ListTrips(c.Context(), companyIDFromCtx(c))
	if err != nil {
		return handleLogisticsError(c, err)
	}
	return c.JSON(trips)
}

func (h *Handler) GetTrip(c *fiber.Ctx) error {
	trip, err := h.svc.GetTrip(c.Context(), companyIDFromCtx(c), c.Params("id"))
	if err != nil {
		return handleLogisticsError(c, err)
	}
	return c.JSON(trip)
}

func (h *Handler) ListFuelIndexes(c *fiber.Ctx) error {
	list, err := h.svc.ListFuelIndexes(c.Context())
	if err != nil {
		return handleLogisticsError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) Project(c *fiber.Ctx) error {
	var req ProjectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	projection, err := h.svc.Project(c.Context(), companyIDFromCtx(c), req)
	if err != nil {
		return handleLogisticsError(c, err)
	}
	return c.JSON(projection)
}

// createTripRequest mirrors what the scheduler sends. contingency_budget is
// accepted and ignored: the cushion is sized by routecost from the route's
// risk profile, and the response carries the real figure.
type createTripRequest struct {
	VehicleID            string  `json:"vehicle_id"`
	RouteID              string  `json:"route_id"`
	ClientID             string  `json:"client_id"`
	ContractID           *string `json:"contract_id"`
	TrackingCode         string  `json:"tracking_code"`
	CargoType            string  `json:"cargo_type"`
	CargoWeightTons      float64 `json:"cargo_weight_tons"`
	DepartureDate        string  `json:"departure_date"`
	EstimatedArrivalDate string  `json:"estimated_arrival_date"`
	AgreedFreightPrice   float64 `json:"agreed_freight_price"`
	Currency             string  `json:"currency"`
	FuelSurchargeAmount  float64 `json:"fuel_surcharge_amount"`
	ContingencyBudget    float64 `json:"contingency_budget"`
}

func (h *Handler) CreateTrip(c *fiber.Ctx) error {
	var req createTripRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	departure, err := parseTimestamp(req.DepartureDate)
	if err != nil {
		return handleLogisticsError(c, ErrInvalidDates)
	}
	arrival, err := parseTimestamp(req.EstimatedArrivalDate)
	if err != nil {
		return handleLogisticsError(c, ErrInvalidDates)
	}

	trip, err := h.svc.CreateTrip(c.Context(), companyIDFromCtx(c), CreateTripInput{
		VehicleID: req.VehicleID, RouteID: req.RouteID, ClientID: req.ClientID, ContractID: req.ContractID,
		TrackingCode: req.TrackingCode, CargoType: req.CargoType, CargoWeightTons: req.CargoWeightTons,
		DepartureDate: departure, EstimatedArrivalDate: arrival,
		AgreedFreightPrice: req.AgreedFreightPrice, Currency: req.Currency,
		FuelSurchargeAmount: req.FuelSurchargeAmount,
	})
	if err != nil {
		return handleLogisticsError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(trip)
}

type advanceSimulationRequest struct {
	TripID *string `json:"trip_id"`
}

func (h *Handler) AdvanceSimulation(c *fiber.Ctx) error {
	var req advanceSimulationRequest
	// An empty body is valid: advance every open trip.
	_ = c.BodyParser(&req)

	tripID := ""
	if req.TripID != nil {
		tripID = *req.TripID
	}
	result, err := h.svc.AdvanceSimulation(c.Context(), companyIDFromCtx(c), tripID)
	if err != nil {
		return handleLogisticsError(c, err)
	}
	return c.JSON(result)
}
