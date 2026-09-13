package fuel

import (
	"context"
	"math"
)

// validFuelTypes mirrors the CHECK constraint on fuel_indexes/trip_fuel_logs/
// fuel_surcharge_rules/route_margin_impacts (000014, 000021).
var validFuelTypes = map[string]bool{
	"diesel":        true,
	"bunker_c":      true,
	"marine_gasoil": true,
	"jet_a1":        true,
}

type Service struct {
	indexes FuelIndexRepository
	logs    TripFuelLogRepository
	rules   FuelSurchargeRuleRepository
	margins RouteMarginImpactRepository
}

func NewService(indexes FuelIndexRepository, logs TripFuelLogRepository, rules FuelSurchargeRuleRepository, margins RouteMarginImpactRepository) *Service {
	return &Service{indexes: indexes, logs: logs, rules: rules, margins: margins}
}

// ---- Fuel indexes ----
// Recording a reading here simulates what a scheduled sync against a real
// provider (EIA/Platts/regional index) would push automatically; no such
// integration exists yet, same stopgap as treasury's manual bank feed. Only
// platform_admin can write these (see server.go) because the table is
// global, not scoped to a company.

func (s *Service) RecordFuelIndex(ctx context.Context, idx *FuelIndex) error {
	if !validFuelTypes[idx.FuelType] {
		return ErrInvalidFuelType
	}
	return s.indexes.CreateFuelIndex(ctx, idx)
}

func (s *Service) ListFuelIndexes(ctx context.Context, fuelType, region string, limit int) ([]FuelIndex, error) {
	return s.indexes.ListRecentFuelIndexes(ctx, fuelType, region, limit)
}

// ListFuelPriceWeeklyTrend reads the fuel_price_weekly continuous aggregate
// (000019) instead of scanning raw fuel_indexes history — a precomputed
// weekly avg/min/max per (fuel_type, region) that Timescale refreshes on
// its own schedule.
func (s *Service) ListFuelPriceWeeklyTrend(ctx context.Context, fuelType, region string, limit int) ([]FuelPriceWeeklyTrend, error) {
	return s.indexes.ListFuelPriceWeeklyTrend(ctx, fuelType, region, limit)
}

// ---- Trip fuel logs ----

func (s *Service) CreateTripFuelLog(ctx context.Context, l *TripFuelLog) error {
	if !validFuelTypes[l.FuelType] {
		return ErrInvalidFuelType
	}
	return s.logs.CreateTripFuelLog(ctx, l)
}

func (s *Service) ListTripFuelLogs(ctx context.Context, companyID string) ([]TripFuelLog, error) {
	return s.logs.ListTripFuelLogsByCompany(ctx, companyID)
}

func (s *Service) ListTripFuelLogsForTrip(ctx context.Context, companyID, tripID string) ([]TripFuelLog, error) {
	return s.logs.ListTripFuelLogsByTrip(ctx, companyID, tripID)
}

func (s *Service) DeleteTripFuelLog(ctx context.Context, companyID, id string) error {
	return s.logs.DeleteTripFuelLog(ctx, companyID, id)
}

// ---- Fuel surcharge rules ----

func (s *Service) CreateSurchargeRule(ctx context.Context, r *FuelSurchargeRule) error {
	if !validFuelTypes[r.FuelType] {
		return ErrInvalidFuelType
	}
	if r.ThresholdPercentage < 0 {
		r.ThresholdPercentage = 0
	}
	if r.PassThroughRate <= 0 {
		r.PassThroughRate = 100
	}
	return s.rules.CreateSurchargeRule(ctx, r)
}

func (s *Service) GetSurchargeRule(ctx context.Context, companyID, id string) (*FuelSurchargeRule, error) {
	return s.rules.GetSurchargeRuleByID(ctx, companyID, id)
}

func (s *Service) ListSurchargeRules(ctx context.Context, companyID string) ([]FuelSurchargeRule, error) {
	return s.rules.ListSurchargeRulesByCompany(ctx, companyID)
}

func (s *Service) UpdateSurchargeRule(ctx context.Context, r *FuelSurchargeRule) error {
	return s.rules.UpdateSurchargeRule(ctx, r)
}

func (s *Service) DeleteSurchargeRule(ctx context.Context, companyID, id string) error {
	return s.rules.DeleteSurchargeRule(ctx, companyID, id)
}

// RecalculateSurchargeRules is the "batch job" logic (mirrors
// treasury.RecalculateForecast / the route_risk_profiles refresh): for every
// active rule of the company, look up the latest fuel_indexes reading for
// that rule's (fuel_type, region), compare it against the rule's baseline
// price, and persist the variation + suggested surcharge.
//
// Surcharge formula: variation_pct = (latest - baseline) / baseline * 100.
// The company absorbs variation up to threshold_percentage on its own
// (that's the margin already built into pricing); only the excess over the
// threshold is recommended as a surcharge, scaled by pass_through_rate (the
// fraction of that excess passed on to the client). A price drop, or a rise
// within the threshold, yields a suggested surcharge of 0 — this never
// recommends a negative surcharge (a rebate), only mitigation of increases.
func (s *Service) RecalculateSurchargeRules(ctx context.Context, companyID string) error {
	rules, err := s.rules.ListActiveSurchargeRulesByCompany(ctx, companyID)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		latest, err := s.indexes.GetLatestFuelIndex(ctx, rule.FuelType, rule.Region)
		if err != nil {
			return err
		}
		if latest == nil {
			continue // no index reading yet for this scope; leave the rule as-is
		}

		variationPct := 0.0
		if rule.BaselinePrice != 0 {
			variationPct = (latest.PricePerUnit - rule.BaselinePrice) / rule.BaselinePrice * 100
		}

		suggested := 0.0
		if excess := variationPct - rule.ThresholdPercentage; excess > 0 {
			suggested = excess * (rule.PassThroughRate / 100)
		}

		price := latest.PricePerUnit
		if err := s.rules.UpdateSurchargeRuleCalculation(ctx, rule.ID, &price, roundTo2(variationPct), roundTo2(suggested)); err != nil {
			return err
		}
	}

	return nil
}

// ---- Route margin impact simulator ----

// SimulateRouteMarginImpact answers "what happens to this route's margin if
// this fuel type's price moves priceVariationPercentage%?" using the route's
// own historical data: the average per-trip fuel spend (from trip_fuel_logs)
// and the average agreed freight price (the revenue baseline). It requires
// at least one trip with a logged fuel purchase of that type on that route —
// with no history, there's nothing honest to extrapolate from, so it returns
// ErrInsufficientData rather than assuming a consumption rate.
func (s *Service) SimulateRouteMarginImpact(ctx context.Context, companyID, routeID, fuelType string, priceVariationPercentage float64) (*RouteMarginImpact, error) {
	if !validFuelTypes[fuelType] {
		return nil, ErrInvalidFuelType
	}
	if math.IsNaN(priceVariationPercentage) || math.IsInf(priceVariationPercentage, 0) {
		return nil, ErrInvalidVariation
	}

	avgFuelCost, avgRevenue, tripCount, err := s.margins.AggregateRouteFuelAndRevenue(ctx, companyID, routeID, fuelType)
	if err != nil {
		return nil, err
	}
	if tripCount == 0 {
		return nil, ErrInsufficientData
	}

	simulatedFuelCost := avgFuelCost * (1 + priceVariationPercentage/100)
	costIncrease := simulatedFuelCost - avgFuelCost

	marginImpactPct := 0.0
	if avgRevenue != 0 {
		marginImpactPct = -(costIncrease / avgRevenue) * 100
	}

	impact := &RouteMarginImpact{
		CompanyID:                         companyID,
		RouteID:                           routeID,
		FuelType:                          fuelType,
		BaselineAvgFuelCost:               roundTo2(avgFuelCost),
		BaselineAvgRevenue:                roundTo2(avgRevenue),
		SimulatedPriceVariationPercentage: roundTo2(priceVariationPercentage),
		SimulatedFuelCost:                 roundTo2(simulatedFuelCost),
		CostIncrease:                      roundTo2(costIncrease),
		MarginImpactPercentage:            roundTo2(marginImpactPct),
		SampleTripCount:                   tripCount,
	}

	if err := s.margins.UpsertRouteMarginImpact(ctx, impact); err != nil {
		return nil, err
	}
	return impact, nil
}

func (s *Service) GetRouteMarginImpact(ctx context.Context, companyID, routeID, fuelType string) (*RouteMarginImpact, error) {
	return s.margins.GetRouteMarginImpact(ctx, companyID, routeID, fuelType)
}

func (s *Service) ListRouteMarginImpacts(ctx context.Context, companyID string) ([]RouteMarginImpact, error) {
	return s.margins.ListRouteMarginImpactsByCompany(ctx, companyID)
}

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}
