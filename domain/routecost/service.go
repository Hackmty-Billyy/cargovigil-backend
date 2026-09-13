package routecost

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"time"
)

const (
	// RiskWindowDays is the rolling window the route score looks at. It is a
	// window and not the whole history on purpose: a route that was terrible
	// two years ago and has been clean since should stop being punished for it.
	RiskWindowDays = 180

	// DefaultRiskScore / DefaultContingencyPercentage are what a route with no
	// history gets — the same neutral values the table defaults to.
	DefaultRiskScore             = 1.00
	DefaultContingencyPercentage = 5.00

	// The suggested cushion spans 3% (a spotless route) to 25% (the worst
	// score this formula can produce).
	minContingencyPercentage = 3.00
	maxContingencyPercentage = 25.00

	// minPlannedHours keeps the derived hourly rate finite when a trip is
	// planned with a near-zero duration (or bad data).
	minPlannedHours = 1.0
)

var tripStatuses = map[string]bool{
	"scheduled":  true,
	"in_transit": true,
	"delayed":    true,
	"completed":  true,
	"cancelled":  true,
}

// closedStatuses are the two end states: reaching either one releases the
// leftover cushion and re-scores the route.
var closedStatuses = map[string]bool{"completed": true, "cancelled": true}

var frictionEventTypes = map[string]bool{
	"port_demurrage":      true,
	"customs_delay":       true,
	"traffic_congestion":  true,
	"mechanical_failure":  true,
	"route_deviation":     true,
	"weather_hazard":      true,
	"security_incident":   true,
	"warehouse_detention": true,
}

// expenseCategoryByEventType maps a physical event to the accounting category
// the resulting payable lands in, so friction costs are not all dumped into
// "other" in the treasury ledger.
var expenseCategoryByEventType = map[string]string{
	"port_demurrage":      "port_fees",
	"customs_delay":       "port_fees",
	"warehouse_detention": "port_fees",
	"mechanical_failure":  "maintenance",
}

type Service struct {
	trips    TripRepository
	friction FrictionRepository
	risk     RiskProfileRepository
	funds    ContingencyFundRepository
	settings CostSettingsRepository
	fx       FXRates
}

func NewService(trips TripRepository, friction FrictionRepository, risk RiskProfileRepository,
	funds ContingencyFundRepository, settings CostSettingsRepository, fx FXRates) *Service {
	return &Service{trips: trips, friction: friction, risk: risk, funds: funds, settings: settings, fx: fx}
}

// ---- Trips ----

func (s *Service) CreateTrip(ctx context.Context, t *Trip) (*Trip, error) {
	if t.Status == "" {
		t.Status = "scheduled"
	}
	if !tripStatuses[t.Status] {
		return nil, ErrInvalidStatus
	}
	if !t.EstimatedArrivalDate.After(t.DepartureDate) {
		return nil, ErrInvalidDates
	}
	if t.Currency == "" {
		t.Currency = TreasuryCurrency
	}
	// The cushion is never taken from the request: it is derived from the
	// route's risk history right after the trip exists.
	t.ContingencyBudget = 0

	if err := s.trips.CreateTrip(ctx, t); err != nil {
		return nil, err
	}

	// "Automatic cushion assignment" from the module spec. A failure here must
	// not undo a valid trip — the allocation is retryable through
	// POST /routecost/trips/:id/contingency/allocate.
	if _, err := s.AllocateContingency(ctx, t.CompanyID, t.ID); err != nil {
		log.Printf("routecost: automatic contingency allocation failed for trip %s: %v", t.ID, err)
		return t, nil
	}
	return s.trips.GetTripByID(ctx, t.CompanyID, t.ID)
}

func (s *Service) GetTrip(ctx context.Context, companyID, id string) (*Trip, error) {
	return s.trips.GetTripByID(ctx, companyID, id)
}

func (s *Service) ListTrips(ctx context.Context, companyID string, filter TripFilter) ([]Trip, error) {
	if filter.Status != "" && !tripStatuses[filter.Status] {
		return nil, ErrInvalidStatus
	}
	return s.trips.ListTripsByCompany(ctx, companyID, filter)
}

func (s *Service) UpdateTrip(ctx context.Context, t *Trip) error {
	if !t.EstimatedArrivalDate.After(t.DepartureDate) {
		return ErrInvalidDates
	}
	if t.Currency == "" {
		t.Currency = TreasuryCurrency
	}
	return s.trips.UpdateTrip(ctx, t)
}

func (s *Service) DeleteTrip(ctx context.Context, companyID, id string) error {
	return s.trips.DeleteTrip(ctx, companyID, id)
}

// ChangeTripStatus is where Module 2 feeds back into modules 1 and 5: closing
// a trip frees the unspent cushion (so the forecast stops holding money that
// is no longer at risk) and re-scores the route with what actually happened.
func (s *Service) ChangeTripStatus(ctx context.Context, companyID, id, status string, actualArrival *time.Time) (*Trip, error) {
	if !tripStatuses[status] {
		return nil, ErrInvalidStatus
	}

	current, err := s.trips.GetTripByID(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	if closedStatuses[current.Status] {
		return nil, ErrInvalidTransition
	}
	if status == "completed" && actualArrival == nil && current.ActualArrivalDate == nil {
		now := time.Now()
		actualArrival = &now
	}

	trip, err := s.trips.UpdateTripStatus(ctx, companyID, id, status, actualArrival)
	if err != nil {
		return nil, err
	}

	if closedStatuses[status] {
		if _, err := s.funds.ReleaseFund(ctx, companyID, id); err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if _, err := s.RecalculateRouteRisk(ctx, companyID, trip.RouteID); err != nil {
			log.Printf("routecost: risk recalculation failed for route %s: %v", trip.RouteID, err)
		}
	}
	return trip, nil
}

// ---- Frictions (downtime events) ----

// FrictionInput is a manual capture of downtime. There is no GPS/weather feed
// wired in yet, so operations register these by hand exactly like treasury
// movements. EndedAt is optional: an event with no end is still running and
// its idle cost is computed live by TripImpact.
type FrictionInput struct {
	TripID       string
	EventType    string
	LocationName *string
	StartedAt    time.Time
	EndedAt      *time.Time
	CostImpact   float64
	Notes        *string
}

func (s *Service) RegisterFriction(ctx context.Context, companyID string, in FrictionInput) (*Friction, error) {
	if !frictionEventTypes[in.EventType] {
		return nil, ErrInvalidEventType
	}
	trip, err := s.trips.GetTripByID(ctx, companyID, in.TripID)
	if err != nil {
		return nil, err
	}
	if in.EndedAt != nil && in.EndedAt.Before(in.StartedAt) {
		return nil, ErrInvalidDates
	}

	f := &Friction{
		CompanyID:    companyID,
		TripID:       in.TripID,
		EventType:    in.EventType,
		LocationName: in.LocationName,
		StartedAt:    in.StartedAt,
		EndedAt:      in.EndedAt,
		CostImpact:   round2(in.CostImpact),
		Notes:        in.Notes,
	}
	if in.EndedAt != nil {
		f.DurationHours = idleHours(in.StartedAt, *in.EndedAt)
		f.OpportunityCost = round2(f.DurationHours * s.idleHourlyRate(ctx, companyID, trip))
	}

	settlement, err := s.buildSettlement(trip, f.EventType, f.CostImpact, f.StartedAt, f.EndedAt)
	if err != nil {
		return nil, err
	}
	if err := s.friction.CreateFriction(ctx, f, settlement); err != nil {
		return nil, err
	}
	return f, nil
}

// CloseFriction stops the clock on a downtime event and books its cost. Only
// the *increase* in cost_impact generates a new payable: closing an event that
// was already registered with a cost must not charge it twice.
func (s *Service) CloseFriction(ctx context.Context, companyID, frictionID string, endedAt time.Time, costImpact *float64, notes *string) (*Friction, error) {
	f, err := s.friction.GetFrictionByID(ctx, companyID, frictionID)
	if err != nil {
		return nil, err
	}
	if f.EndedAt != nil {
		return nil, ErrFrictionClosed
	}
	if endedAt.Before(f.StartedAt) {
		return nil, ErrInvalidDates
	}
	trip, err := s.trips.GetTripByID(ctx, companyID, f.TripID)
	if err != nil {
		return nil, err
	}

	previousCost := f.CostImpact
	if costImpact != nil {
		f.CostImpact = round2(*costImpact)
	}
	f.EndedAt = &endedAt
	f.DurationHours = idleHours(f.StartedAt, endedAt)
	f.OpportunityCost = round2(f.DurationHours * s.idleHourlyRate(ctx, companyID, trip))
	if notes != nil {
		f.Notes = notes
	}

	settlement, err := s.buildSettlement(trip, f.EventType, round2(f.CostImpact-previousCost), f.StartedAt, f.EndedAt)
	if err != nil {
		return nil, err
	}
	if err := s.friction.CloseFriction(ctx, f, settlement); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Service) ListFrictionsByTrip(ctx context.Context, companyID, tripID string) ([]Friction, error) {
	return s.friction.ListFrictionsByTrip(ctx, companyID, tripID)
}

func (s *Service) ListFrictions(ctx context.Context, companyID string, onlyOpen bool) ([]Friction, error) {
	return s.friction.ListFrictionsByCompany(ctx, companyID, onlyOpen)
}

// buildSettlement converts a friction's out-of-pocket cost into the payable
// treasury will see. Amounts on a trip are in the trip's currency; treasury
// reasons in MXN only, so the conversion is mandatory, not cosmetic.
func (s *Service) buildSettlement(trip *Trip, eventType string, costImpact float64, startedAt time.Time, endedAt *time.Time) (*FrictionSettlement, error) {
	if costImpact <= 0 {
		return nil, nil
	}
	amount, _, err := s.fx.Convert(costImpact, trip.Currency, TreasuryCurrency)
	if err != nil {
		return nil, err
	}
	category, ok := expenseCategoryByEventType[eventType]
	if !ok {
		category = "other"
	}
	dueDate := startedAt
	if endedAt != nil {
		dueDate = *endedAt
	}
	return &FrictionSettlement{
		ExpenseCategory:    category,
		ExpenseDescription: fmt.Sprintf("Fricción %s en viaje %s", eventType, trip.TrackingCode),
		ExpenseAmount:      amount,
		ExpenseDueDate:     dueDate,
	}, nil
}

// idleHourlyRate answers "what is one idle hour worth on this trip", in the
// trip's currency. The per-company override wins when it is set; otherwise the
// rate is derived from the trip itself — the freight the asset is paid divided
// by the hours it was supposed to take — which needs no configuration and is
// defensible to a client.
func (s *Service) idleHourlyRate(ctx context.Context, companyID string, trip *Trip) float64 {
	settings, err := s.settings.GetCostSettings(ctx, companyID)
	if err == nil && settings.IdleHourlyRate != nil && *settings.IdleHourlyRate > 0 {
		if rate, _, convErr := s.fx.Convert(*settings.IdleHourlyRate, settings.Currency, trip.Currency); convErr == nil {
			return rate
		}
		// Unsupported currency pair: fall back to the trip-derived rate rather
		// than silently applying a number in the wrong currency.
	}
	if trip.AgreedFreightPrice <= 0 {
		return 0
	}
	planned := trip.EstimatedArrivalDate.Sub(trip.DepartureDate).Hours()
	if planned < minPlannedHours {
		planned = minPlannedHours
	}
	return round2(trip.AgreedFreightPrice / planned)
}

func idleHours(from, to time.Time) float64 {
	h := to.Sub(from).Hours()
	if h < 0 {
		return 0
	}
	return round2(h)
}

// ---- Route risk ----

func (s *Service) ListRiskProfiles(ctx context.Context, companyID string) ([]RouteRiskProfile, error) {
	return s.risk.ListRiskProfilesByCompany(ctx, companyID)
}

// GetRouteRisk returns the stored profile of one route, or the neutral
// defaults when the route has no history yet — so callers pricing a trip
// (domain/logistics' pre-trip projection) never have to special-case a route
// that was just created.
func (s *Service) GetRouteRisk(ctx context.Context, companyID, routeID string) (*RouteRiskProfile, error) {
	profile, err := s.risk.GetRiskProfile(ctx, companyID, routeID)
	if errors.Is(err, ErrNotFound) {
		return &RouteRiskProfile{
			CompanyID:                      companyID,
			RouteID:                        routeID,
			HistoricalRiskScore:            DefaultRiskScore,
			SuggestedContingencyPercentage: DefaultContingencyPercentage,
		}, nil
	}
	return profile, err
}

// RecalculateRouteRisk scores one route from the last RiskWindowDays days.
//
//	score = 1 + delay factor + incident factor, each capped at 1.0
//	  delay factor    = avg delay hours / 24      (a full day late = worst)
//	  incident factor = frictions per trip / 2    (2+ frictions per trip = worst)
//
// so the score lands in 1.00 (clean) .. 3.00 (worst), which matches the
// multiplier-style values already seeded in route_risk_profiles. The suggested
// cushion is a straight line from that: 3% at score 1 to 25% at score 3.
func (s *Service) RecalculateRouteRisk(ctx context.Context, companyID, routeID string) (*RouteRiskProfile, error) {
	since := time.Now().AddDate(0, 0, -RiskWindowDays)
	stats, err := s.risk.RiskStatsForRoute(ctx, companyID, routeID, since)
	if err != nil {
		return nil, err
	}

	score := DefaultRiskScore
	avgDelay := 0.0
	if stats.TripCount > 0 {
		avgDelay = round2(stats.AvgDelayHours)
		delayFactor := math.Min(1.0, avgDelay/24.0)
		incidentFactor := math.Min(1.0, (float64(stats.IncidentCount)/float64(stats.TripCount))/2.0)
		score = round2(1.0 + delayFactor + incidentFactor)
	}

	profile := &RouteRiskProfile{
		CompanyID:                      companyID,
		RouteID:                        routeID,
		HistoricalRiskScore:            score,
		AvgDelayHours:                  avgDelay,
		SuggestedContingencyPercentage: contingencyPercentageFor(score),
		IncidentCount:                  stats.IncidentCount,
	}
	if err := s.risk.UpsertRiskProfile(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *Service) RecalculateAllRouteRisks(ctx context.Context, companyID string) ([]RouteRiskProfile, error) {
	routeIDs, err := s.trips.ListRouteIDsByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	profiles := []RouteRiskProfile{}
	for _, routeID := range routeIDs {
		p, err := s.RecalculateRouteRisk(ctx, companyID, routeID)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}
	return profiles, nil
}

func contingencyPercentageFor(score float64) float64 {
	pct := minContingencyPercentage + (score-1.0)*11.0
	if pct < minContingencyPercentage {
		pct = minContingencyPercentage
	}
	if pct > maxContingencyPercentage {
		pct = maxContingencyPercentage
	}
	return round2(pct)
}

// ---- Contingency funds ----

// AllocateContingency sizes the liquidity cushion of a trip from its route's
// risk profile and mirrors it into the cash flow forecast as a pending
// contingency_reserve expense.
func (s *Service) AllocateContingency(ctx context.Context, companyID, tripID string) (*ContingencyFund, error) {
	trip, err := s.trips.GetTripByID(ctx, companyID, tripID)
	if err != nil {
		return nil, err
	}

	score, percentage := DefaultRiskScore, DefaultContingencyPercentage
	profile, err := s.risk.GetRiskProfile(ctx, companyID, trip.RouteID)
	switch {
	case err == nil:
		score, percentage = profile.HistoricalRiskScore, profile.SuggestedContingencyPercentage
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}

	budgetInTripCurrency := round2(trip.AgreedFreightPrice * percentage / 100.0)
	allocated, rate, err := s.fx.Convert(budgetInTripCurrency, trip.Currency, TreasuryCurrency)
	if err != nil {
		return nil, err
	}

	fund := &ContingencyFund{
		CompanyID:         companyID,
		TripID:            tripID,
		RouteRiskScore:    score,
		AppliedPercentage: percentage,
		BaseAmount:        trip.AgreedFreightPrice,
		BaseCurrency:      trip.Currency,
		FXRate:            rate,
		ReserveCurrency:   TreasuryCurrency,
		AllocatedAmount:   allocated,
	}
	reserve := ReserveExpense{
		Description: fmt.Sprintf("Colchón de contingencia viaje %s (%.2f%% por riesgo de ruta)", trip.TrackingCode, percentage),
		Amount:      allocated,
		DueDate:     reserveDueDate(trip.DepartureDate),
	}
	return s.funds.AllocateFund(ctx, fund, reserve, budgetInTripCurrency)
}

// reserveDueDate keeps the reserve inside the forward-looking window: the
// forecast walks from tomorrow, so a cushion dated in the past would never be
// seen by it.
func reserveDueDate(departure time.Time) time.Time {
	tomorrow := time.Now().AddDate(0, 0, 1)
	if departure.Before(tomorrow) {
		return tomorrow
	}
	return departure
}

// AllocateContingencyForOpenTrips sizes a cushion for every open trip that
// never got one. Trips that arrive by import or by the seeder write
// trips.contingency_budget directly, so they have the number but none of the
// fund's accounting — this is the catch-up pass. One failing trip does not
// abort the rest; the count says how many actually landed.
func (s *Service) AllocateContingencyForOpenTrips(ctx context.Context, companyID string) (int, error) {
	tripIDs, err := s.funds.ListTripIDsWithoutFund(ctx, companyID)
	if err != nil {
		return 0, err
	}

	allocated := 0
	for _, tripID := range tripIDs {
		if _, err := s.AllocateContingency(ctx, companyID, tripID); err != nil {
			log.Printf("routecost: bulk allocation skipped trip %s: %v", tripID, err)
			continue
		}
		allocated++
	}
	return allocated, nil
}

func (s *Service) ReleaseContingency(ctx context.Context, companyID, tripID string) (*ContingencyFund, error) {
	return s.funds.ReleaseFund(ctx, companyID, tripID)
}

func (s *Service) GetContingency(ctx context.Context, companyID, tripID string) (*ContingencyFund, error) {
	return s.funds.GetFundByTrip(ctx, companyID, tripID)
}

func (s *Service) ListContingencyFunds(ctx context.Context, companyID string) ([]ContingencyFund, error) {
	return s.funds.ListFundsByCompany(ctx, companyID)
}

// ---- Impact read model ----

// TripImpact translates the physical inefficiency of a trip into money: idle
// hours, what they cost out of pocket, what they cost in revenue the asset
// could not produce, and how the cushion is holding up. Open frictions count
// their idle time up to now.
func (s *Service) TripImpact(ctx context.Context, companyID, tripID string) (*TripImpact, error) {
	trip, err := s.trips.GetTripByID(ctx, companyID, tripID)
	if err != nil {
		return nil, err
	}
	frictions, err := s.friction.ListFrictionsByTrip(ctx, companyID, tripID)
	if err != nil {
		return nil, err
	}

	rate := s.idleHourlyRate(ctx, companyID, trip)
	impact := &TripImpact{TripID: tripID, Currency: trip.Currency, IdleHourlyRate: rate}

	now := time.Now()
	for _, f := range frictions {
		impact.FrictionCount++
		impact.DirectCost = round2(impact.DirectCost + f.CostImpact)

		if f.EndedAt == nil {
			impact.OpenFrictionCount++
			live := idleHours(f.StartedAt, now)
			impact.TotalIdleHours = round2(impact.TotalIdleHours + live)
			impact.OpportunityCost = round2(impact.OpportunityCost + live*rate)
			continue
		}
		impact.TotalIdleHours = round2(impact.TotalIdleHours + f.DurationHours)
		impact.OpportunityCost = round2(impact.OpportunityCost + f.OpportunityCost)
	}
	impact.TotalFrictionImpact = round2(impact.DirectCost + impact.OpportunityCost)

	fund, err := s.funds.GetFundByTrip(ctx, companyID, tripID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	impact.Fund = fund
	return impact, nil
}

// ---- Cost settings ----

func (s *Service) GetCostSettings(ctx context.Context, companyID string) (*CostSettings, error) {
	settings, err := s.settings.GetCostSettings(ctx, companyID)
	if errors.Is(err, ErrNotFound) {
		// No override configured: report the effective default explicitly
		// instead of 404-ing, so the dashboard can render the state.
		return &CostSettings{CompanyID: companyID, IdleHourlyRate: nil, Currency: TreasuryCurrency}, nil
	}
	return settings, err
}

func (s *Service) UpdateCostSettings(ctx context.Context, cfg *CostSettings) error {
	if cfg.Currency == "" {
		cfg.Currency = TreasuryCurrency
	}
	if cfg.IdleHourlyRate != nil && *cfg.IdleHourlyRate < 0 {
		return ErrInvalidRate
	}
	return s.settings.UpsertCostSettings(ctx, cfg)
}
