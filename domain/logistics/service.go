package logistics

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/fuel"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/routecost"
)

// fuelTypeByVehicle picks which index of module 3 prices this asset.
var fuelTypeByVehicle = map[string]string{
	"truck": "diesel",
	"ship":  "bunker_c",
	"plane": "jet_a1",
}

// fuelIndexCurrency: fuel_indexes has no currency column, and the seeded
// sources are mixed — the Mexican regulator publishes pesos per litre while
// bunker/jet indexes are quoted in dollars per metric ton. Until that column
// exists, this map is the single place that assumption lives, instead of
// letting it leak into every calculation.
var fuelIndexCurrency = map[string]string{
	"diesel":        "MXN",
	"jet_a1":        "MXN",
	"bunker_c":      "USD",
	"marine_gasoil": "USD",
}

// Consumption per kilometre at empty weight, in the unit the index is quoted
// in (litres for diesel/jet, metric tons for bunker), plus how much each ton
// of cargo adds. Rough but stable figures — the point is that a heavier load
// on a longer route costs visibly more, not to model an engine.
var consumptionPerKM = map[string]float64{"truck": 0.35, "ship": 0.028, "plane": 3.10}
var loadFactorPerTon = map[string]float64{"truck": 0.0045, "ship": 0.0000035, "plane": 0.0120}

// Cruising speeds used to turn a distance into planned hours, which is what
// an idle hour is priced against (same idea as routecost's derived rate).
var avgSpeedKMH = map[string]float64{"truck": 70, "ship": 33, "plane": 780}

// What can go wrong on the road, and which module-2 friction it registers.
var stuckEvents = []struct {
	Reason    string
	EventType string
	CostRate  float64 // share of the freight the incident ends up costing
}{
	{"Retención en aduana", "customs_delay", 0.030},
	{"Congestión y espera de atraque en puerto", "port_demurrage", 0.045},
	{"Falla mecánica en unidad", "mechanical_failure", 0.055},
	{"Bloqueo carretero", "traffic_congestion", 0.020},
	{"Clima adverso en ruta", "weather_hazard", 0.025},
	{"Detención en centro de distribución", "warehouse_detention", 0.022},
}

const (
	// One simulation step is roughly four hours of road time.
	stepHours = 4.0

	minProgressStep  = 8.0
	maxProgressStep  = 22.0
	stuckProbability = 0.18
	unstuckChance    = 0.60
)

type Service struct {
	repo      Repository
	routeCost *routecost.Service
	fuel      *fuel.Service
	fx        routecost.FXRates
}

func NewService(repo Repository, routeCost *routecost.Service, fuelSvc *fuel.Service, fx routecost.FXRates) *Service {
	return &Service{repo: repo, routeCost: routeCost, fuel: fuelSvc, fx: fx}
}

// ---- Reads ----

func (s *Service) ListTrips(ctx context.Context, companyID string) ([]Trip, error) {
	return s.repo.ListTrips(ctx, companyID)
}

func (s *Service) GetTrip(ctx context.Context, companyID, id string) (*Trip, error) {
	return s.repo.GetTrip(ctx, companyID, id)
}

func (s *Service) DeleteTrip(ctx context.Context, companyID, id string) error {
	return s.routeCost.DeleteTrip(ctx, companyID, id)
}

func (s *Service) ListFuelIndexes(ctx context.Context) ([]fuel.FuelIndex, error) {
	return s.fuel.ListFuelIndexes(ctx, "", "", 50)
}

// ---- Pre-trip projection ----

// Project answers "is this trip worth running" before it is committed, by
// combining the three modules that already know something about it: the fuel
// index (module 3), the route's risk history and the cushion it implies
// (module 2). Every amount comes back in the currency the caller is pricing
// in, never mixed.
func (s *Service) Project(ctx context.Context, companyID string, req ProjectionRequest) (*ProjectionResponse, error) {
	fuelType, ok := fuelTypeByVehicle[req.VehicleType]
	if !ok {
		return nil, ErrInvalidVehicle
	}
	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}
	distance := req.DistanceKM
	if distance <= 0 {
		distance = 350
	}

	indexes, err := s.fuel.ListFuelIndexes(ctx, fuelType, "", 1)
	if err != nil {
		return nil, err
	}
	if len(indexes) == 0 {
		return nil, ErrNoFuelIndex
	}
	idx := indexes[0]

	quantity := fuelQuantity(req.VehicleType, distance, req.CargoWeightTons)
	fuelCost, _, err := s.fx.Convert(quantity*idx.PricePerUnit, fuelIndexCurrency[fuelType], currency)
	if err != nil {
		return nil, ErrInvalidCurrency
	}

	profile, err := s.routeCost.GetRouteRisk(ctx, companyID, req.RouteID)
	if err != nil {
		return nil, err
	}

	price := req.AgreedPrice
	suggested := round2(price * profile.SuggestedContingencyPercentage / 100)
	hourlyRate := price / plannedHours(req.VehicleType, distance)
	potentialLoss := expectedDelayLoss(profile.AvgDelayHours, hourlyRate, profile.HistoricalRiskScore)

	netMargin := round2(price - fuelCost - potentialLoss)
	marginPct := 0.0
	if price > 0 {
		marginPct = round2(netMargin / price * 100)
	}

	return &ProjectionResponse{
		DistanceKM:            round2(distance),
		FuelType:              idx.FuelType,
		CurrentFuelPrice:      idx.PricePerUnit,
		FuelUnit:              idx.UnitOfMeasure,
		FuelSource:            idx.Source,
		EstimatedFuelQuantity: round2(quantity),
		EstimatedFuelCost:     round2(fuelCost),
		SuggestedContingency:  suggested,
		RiskScore:             profile.HistoricalRiskScore,
		AverageDelayHours:     profile.AvgDelayHours,
		PotentialLossRisk:     potentialLoss,
		ProjectedNetMargin:    netMargin,
		ProjectedMarginPct:    marginPct,
		Recommendation:        recommendationFor(marginPct, profile.HistoricalRiskScore),
	}, nil
}

func recommendationFor(marginPct, riskScore float64) string {
	switch {
	case marginPct < 0:
		return "No recomendado: el viaje proyecta pérdida con el flete actual. Renegocia el precio o cambia de ruta."
	case marginPct < 10:
		return "Margen ajustado. Cualquier retraso se come la utilidad; considera subir el flete."
	case riskScore >= 2.0:
		return "Margen sano pero la ruta es de alto riesgo. Mantén el colchón de contingencia asignado."
	case marginPct < 25:
		return "Margen aceptable. Viaje viable con el colchón calculado."
	default:
		return "Viaje rentable. Proceder con la programación."
	}
}

// ---- Trip creation ----

// CreateTripInput is the scheduler's payload. contingency_budget is
// deliberately absent: the cushion is sized by routecost from the route's risk
// profile, not by the client.
type CreateTripInput struct {
	VehicleID            string
	RouteID              string
	ClientID             string
	ContractID           *string
	TrackingCode         string
	CargoType            string
	CargoWeightTons      float64
	DepartureDate        time.Time
	EstimatedArrivalDate time.Time
	AgreedFreightPrice   float64
	Currency             string
	FuelSurchargeAmount  float64
}

// CreateTrip delegates to routecost so a trip born in the radar is identical
// to one born in /routecost — same validation, same automatic cushion — and
// then seeds its tracking state at the route's origin.
func (s *Service) CreateTrip(ctx context.Context, companyID string, in CreateTripInput) (*Trip, error) {
	created, err := s.routeCost.CreateTrip(ctx, &routecost.Trip{
		CompanyID:            companyID,
		VehicleID:            in.VehicleID,
		RouteID:              in.RouteID,
		ClientID:             in.ClientID,
		ContractID:           in.ContractID,
		TrackingCode:         in.TrackingCode,
		CargoType:            in.CargoType,
		CargoWeightTons:      in.CargoWeightTons,
		DepartureDate:        in.DepartureDate,
		EstimatedArrivalDate: in.EstimatedArrivalDate,
		AgreedFreightPrice:   in.AgreedFreightPrice,
		Currency:             in.Currency,
		FuelSurchargeAmount:  in.FuelSurchargeAmount,
	})
	if err != nil {
		return nil, err
	}

	trip, err := s.repo.GetTrip(ctx, companyID, created.ID)
	if err != nil {
		return nil, err
	}

	trip.CurrentLat, trip.CurrentLng = trip.OriginLat, trip.OriginLng
	trip.EstimatedFuelCost, trip.EstimatedLossRisk = s.estimatesFor(ctx, trip)
	if err := s.repo.UpdateTracking(ctx, trip); err != nil {
		return nil, err
	}
	return trip, nil
}

// ---- Simulation ----

// AdvanceSimulation moves every open trip one step forward: progress along the
// route, the occasional incident, and arrival. It stands in for the GPS feed
// that does not exist yet, and it is wired into the real modules — an incident
// registers a module-2 friction (which consumes the trip's cushion) and an
// arrival goes through routecost, which releases what is left of that cushion
// and re-scores the route.
//
// It returns the full trip list because the radar replaces its state with it.
func (s *Service) AdvanceSimulation(ctx context.Context, companyID, tripID string) (*SimulationResult, error) {
	trips, err := s.repo.ListSimulatableTrips(ctx, companyID, tripID)
	if err != nil {
		return nil, err
	}

	var advanced, stuck, arrived int
	for i := range trips {
		t := &trips[i]
		outcome := s.advanceOne(ctx, t)
		if err := s.repo.UpdateTracking(ctx, t); err != nil {
			log.Printf("logistics: could not persist simulation step for trip %s: %v", t.ID, err)
			continue
		}
		advanced++
		switch outcome {
		case outcomeStuck:
			stuck++
		case outcomeArrived:
			arrived++
		}
	}

	all, err := s.repo.ListTrips(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return &SimulationResult{Message: simulationMessage(advanced, stuck, arrived), Trips: all}, nil
}

type stepOutcome int

const (
	outcomeMoved stepOutcome = iota
	outcomeStuck
	outcomeFreed
	outcomeArrived
	outcomeIdle
)

func (s *Service) advanceOne(ctx context.Context, t *Trip) stepOutcome {
	now := time.Now()
	t.LastSimulatedStep++
	hourlyRate := t.AgreedFreightPrice / plannedHours(t.VehicleType, distanceOf(t))

	switch {
	case t.IsStuck:
		// Still held up: the idle hours keep accumulating against the trip.
		t.EstimatedLossRisk = cappedLoss(t.EstimatedLossRisk+stepHours*hourlyRate, t.AgreedFreightPrice)
		if rand.Float64() > unstuckChance {
			return outcomeIdle
		}
		s.closeOpenFriction(ctx, t)
		t.IsStuck = false
		t.StuckReason = nil
		t.StuckSinceStep = 0
		t.Status = "in_transit"
		return outcomeFreed

	case t.Status == "scheduled":
		t.Status = "in_transit"
		t.CurrentLat, t.CurrentLng = t.OriginLat, t.OriginLng
		t.EstimatedFuelCost, t.EstimatedLossRisk = s.estimatesFor(ctx, t)
		return outcomeMoved
	}

	progress := t.ProgressPercentage + minProgressStep + rand.Float64()*(maxProgressStep-minProgressStep)
	if progress >= 100 {
		t.ProgressPercentage = 100
		t.CurrentLat, t.CurrentLng = t.DestLat, t.DestLng
		t.ActualArrivalDate = &now
		t.Status = "completed"
		// Through routecost so the cushion is released and the route re-scored
		// exactly as it would be from /routecost/trips/:id/status.
		if _, err := s.routeCost.ChangeTripStatus(ctx, t.CompanyID, t.ID, "completed", &now); err != nil {
			log.Printf("logistics: trip %s arrived but routecost close failed: %v", t.ID, err)
		}
		return outcomeArrived
	}

	t.ProgressPercentage = round2(progress)
	t.CurrentLat, t.CurrentLng = interpolate(t, t.ProgressPercentage)

	if rand.Float64() < stuckProbability {
		event := stuckEvents[rand.Intn(len(stuckEvents))]
		reason := event.Reason
		t.IsStuck = true
		t.StuckReason = &reason
		t.Status = "delayed"
		t.StuckSinceStep = t.LastSimulatedStep
		t.EstimatedLossRisk = cappedLoss(t.EstimatedLossRisk+stepHours*hourlyRate, t.AgreedFreightPrice)
		s.openFriction(ctx, t, event.Reason, event.EventType)
		return outcomeStuck
	}

	t.Status = "in_transit"
	return outcomeMoved
}

// openFriction records the incident as a real module-2 downtime event. It is
// best-effort: a failure here must not abort the simulation step that already
// moved the trip.
func (s *Service) openFriction(ctx context.Context, t *Trip, reason, eventType string) {
	location := t.RouteDestination
	if location == "" {
		location = reason
	}
	note := fmt.Sprintf("Incidente detectado en seguimiento en vivo al %.0f%% de la ruta", t.ProgressPercentage)
	if _, err := s.routeCost.RegisterFriction(ctx, t.CompanyID, routecost.FrictionInput{
		TripID:       t.ID,
		EventType:    eventType,
		LocationName: &location,
		StartedAt:    time.Now(),
		Notes:        &note,
	}); err != nil {
		log.Printf("logistics: could not register friction for trip %s: %v", t.ID, err)
	}
}

// closeOpenFriction settles the incident when the trip gets moving again. The
// cost is charged against the contingency cushion by routecost.
func (s *Service) closeOpenFriction(ctx context.Context, t *Trip) {
	frictions, err := s.routeCost.ListFrictionsByTrip(ctx, t.CompanyID, t.ID)
	if err != nil {
		log.Printf("logistics: could not read frictions of trip %s: %v", t.ID, err)
		return
	}
	steps := t.LastSimulatedStep - t.StuckSinceStep
	if steps < 1 {
		steps = 1
	}
	// Simulated, not wall clock: clicking "advance" twice in five seconds
	// still means eight hours stuck at the port.
	held := time.Duration(float64(steps) * stepHours * float64(time.Hour))

	for i := range frictions {
		f := frictions[i]
		if f.EndedAt != nil {
			continue
		}
		cost := round2(t.AgreedFreightPrice * costRateFor(f.EventType))
		if _, err := s.routeCost.CloseFriction(ctx, t.CompanyID, f.ID, f.StartedAt.Add(held), &cost, nil); err != nil {
			log.Printf("logistics: could not close friction %s: %v", f.ID, err)
		}
		return
	}
}

func costRateFor(eventType string) float64 {
	for _, e := range stuckEvents {
		if e.EventType == eventType {
			return e.CostRate
		}
	}
	return 0.02
}

// estimatesFor prices the trip's fuel and its expected loss from delay, both
// in the trip's own currency.
func (s *Service) estimatesFor(ctx context.Context, t *Trip) (fuelCost, lossRisk float64) {
	distance := distanceOf(t)
	lossRisk = expectedDelayLoss(t.RiskAvgDelayH, t.AgreedFreightPrice/plannedHours(t.VehicleType, distance), t.RiskScore)

	fuelType, ok := fuelTypeByVehicle[t.VehicleType]
	if !ok {
		return 0, lossRisk
	}
	indexes, err := s.fuel.ListFuelIndexes(ctx, fuelType, "", 1)
	if err != nil || len(indexes) == 0 {
		return 0, lossRisk
	}
	converted, _, err := s.fx.Convert(
		fuelQuantity(t.VehicleType, distance, t.CargoWeightTons)*indexes[0].PricePerUnit,
		fuelIndexCurrency[fuelType], t.Currency)
	if err != nil {
		return 0, lossRisk
	}
	return round2(converted), lossRisk
}

// ---- helpers ----

func fuelQuantity(vehicleType string, distanceKM, cargoTons float64) float64 {
	base := consumptionPerKM[vehicleType]
	perTon := loadFactorPerTon[vehicleType]
	return (base + perTon*cargoTons) * distanceKM
}

// expectedDelayLoss is the delay exposure the margin is actually charged for.
// The raw figure — every historical delay hour priced at the trip's own
// revenue rate, which is how module 2 values idle time — is the worst case,
// and on short routes it can exceed the whole freight. What a pre-trip
// decision needs is the part expected to materialise, so the exposure is
// scaled by the route's risk score: 25% on a clean route (1.00), 100% on the
// worst one (3.00). Keeping the reported number and the margin consistent
// matters more than reporting the scarier one: the scheduler shows both side
// by side and they have to add up.
func expectedDelayLoss(avgDelayHours, hourlyRate, riskScore float64) float64 {
	exposure := 0.25 + 0.375*(riskScore-1)
	if exposure < 0.25 {
		exposure = 0.25
	}
	if exposure > 1 {
		exposure = 1
	}
	return round2(avgDelayHours * hourlyRate * exposure)
}

func plannedHours(vehicleType string, distanceKM float64) float64 {
	speed := avgSpeedKMH[vehicleType]
	if speed <= 0 {
		speed = 70
	}
	hours := distanceKM / speed
	if hours < 1 {
		return 1
	}
	return hours
}

func distanceOf(t *Trip) float64 {
	if t.RouteDistanceKM != nil && *t.RouteDistanceKM > 0 {
		return *t.RouteDistanceKM
	}
	return 350
}

// interpolate walks the straight line between the route's endpoints. A route
// with no coordinates keeps its last known position instead of jumping to
// (0,0) in the middle of the Atlantic.
func interpolate(t *Trip, progress float64) (*float64, *float64) {
	if t.OriginLat == nil || t.OriginLng == nil || t.DestLat == nil || t.DestLng == nil {
		return t.CurrentLat, t.CurrentLng
	}
	ratio := progress / 100
	lat := *t.OriginLat + (*t.DestLat-*t.OriginLat)*ratio
	lng := *t.OriginLng + (*t.DestLng-*t.OriginLng)*ratio
	return &lat, &lng
}

func simulationMessage(advanced, stuck, arrived int) string {
	if advanced == 0 {
		return "No hay viajes activos que avanzar."
	}
	msg := fmt.Sprintf("%d viaje(s) avanzaron", advanced)
	if stuck > 0 {
		msg += fmt.Sprintf(" · %d incidente(s) nuevo(s)", stuck)
	}
	if arrived > 0 {
		msg += fmt.Sprintf(" · %d llegada(s)", arrived)
	}
	return msg
}

// cappedLoss keeps the radar's exposure figure readable: idle hours priced at
// the trip's revenue rate can genuinely exceed the freight, but as a headline
// number next to that freight it reads as a bug. The uncapped truth stays
// where it belongs — trip_frictions.opportunity_cost, computed by module 2
// with no ceiling.
func cappedLoss(value, freightPrice float64) float64 {
	if freightPrice > 0 && value > freightPrice {
		return round2(freightPrice)
	}
	return round2(value)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
