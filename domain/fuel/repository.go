package fuel

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

// This is the Bun-backed rewrite of what used to be raw pgx SQL. Interfaces
// are unchanged from the original (method names still suffixed per entity —
// CreateTripFuelLog, CreateSurchargeRule, ... — because SimulateRouteMarginImpact
// needs one struct that can read trip_fuel_logs/trips and write
// route_margin_impacts), so domain/fuel/service.go and handler.go did not
// change at all when this swapped from pgx to Bun.
//
// Bun here is a pure query/mapping layer: schema (including the
// TimescaleDB-specific DDL — hypertables, continuous aggregates,
// compression/retention policies) is still owned exclusively by
// golang-migrate. See database/bun.go.

const pgUniqueViolation = "23505"

type FuelIndexRepository interface {
	CreateFuelIndex(ctx context.Context, idx *FuelIndex) error
	ListRecentFuelIndexes(ctx context.Context, fuelType, region string, limit int) ([]FuelIndex, error)
	GetLatestFuelIndex(ctx context.Context, fuelType, region string) (*FuelIndex, error)
	// ListFuelPriceWeeklyTrend reads the fuel_price_weekly continuous
	// aggregate (migration 000019) — precomputed server-side by Timescale,
	// this is a plain SELECT against that view, same as any other model.
	ListFuelPriceWeeklyTrend(ctx context.Context, fuelType, region string, limit int) ([]FuelPriceWeeklyTrend, error)
}

type TripFuelLogRepository interface {
	CreateTripFuelLog(ctx context.Context, l *TripFuelLog) error
	ListTripFuelLogsByCompany(ctx context.Context, companyID string) ([]TripFuelLog, error)
	ListTripFuelLogsByTrip(ctx context.Context, companyID, tripID string) ([]TripFuelLog, error)
	DeleteTripFuelLog(ctx context.Context, companyID, id string) error
}

type FuelSurchargeRuleRepository interface {
	CreateSurchargeRule(ctx context.Context, r *FuelSurchargeRule) error
	GetSurchargeRuleByID(ctx context.Context, companyID, id string) (*FuelSurchargeRule, error)
	ListSurchargeRulesByCompany(ctx context.Context, companyID string) ([]FuelSurchargeRule, error)
	ListActiveSurchargeRulesByCompany(ctx context.Context, companyID string) ([]FuelSurchargeRule, error)
	UpdateSurchargeRule(ctx context.Context, r *FuelSurchargeRule) error
	UpdateSurchargeRuleCalculation(ctx context.Context, id string, lastIndexPrice *float64, variationPct, suggestedPct float64) error
	DeleteSurchargeRule(ctx context.Context, companyID, id string) error
}

type RouteMarginImpactRepository interface {
	AggregateRouteFuelAndRevenue(ctx context.Context, companyID, routeID, fuelType string) (avgFuelCost, avgRevenue float64, tripCount int, err error)
	UpsertRouteMarginImpact(ctx context.Context, m *RouteMarginImpact) error
	GetRouteMarginImpact(ctx context.Context, companyID, routeID, fuelType string) (*RouteMarginImpact, error)
	ListRouteMarginImpactsByCompany(ctx context.Context, companyID string) ([]RouteMarginImpact, error)
}

type BunRepository struct{ db *bun.DB }

func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// ---- Fuel indexes (global, no company_id) ----

func (r *BunRepository) CreateFuelIndex(ctx context.Context, idx *FuelIndex) error {
	if idx.UnitOfMeasure == "" {
		idx.UnitOfMeasure = "liter"
	}
	if idx.Source == "" {
		idx.Source = "manual"
	}
	_, err := r.db.NewInsert().Model(idx).Returning("id, recorded_at").Exec(ctx)
	return err
}

func (r *BunRepository) ListRecentFuelIndexes(ctx context.Context, fuelType, region string, limit int) ([]FuelIndex, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	out := []FuelIndex{}
	q := r.db.NewSelect().Model(&out).OrderExpr("recorded_at DESC").Limit(limit)
	if fuelType != "" {
		q = q.Where("fuel_type = ?", fuelType)
	}
	if region != "" {
		q = q.Where("region = ?", region)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) GetLatestFuelIndex(ctx context.Context, fuelType, region string) (*FuelIndex, error) {
	idx := new(FuelIndex)
	err := r.db.NewSelect().Model(idx).
		Where("fuel_type = ?", fuelType).
		Where("region = ?", region).
		OrderExpr("recorded_at DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return idx, nil
}

func (r *BunRepository) ListFuelPriceWeeklyTrend(ctx context.Context, fuelType, region string, limit int) ([]FuelPriceWeeklyTrend, error) {
	if limit <= 0 || limit > 104 {
		limit = 12
	}
	out := []FuelPriceWeeklyTrend{}
	q := r.db.NewSelect().Model(&out).OrderExpr("bucket DESC").Limit(limit)
	if fuelType != "" {
		q = q.Where("fuel_type = ?", fuelType)
	}
	if region != "" {
		q = q.Where("region = ?", region)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- Trip fuel logs ----

func (r *BunRepository) CreateTripFuelLog(ctx context.Context, l *TripFuelLog) error {
	_, err := r.db.NewInsert().Model(l).Returning("id, purchased_at").Exec(ctx)
	return err
}

func (r *BunRepository) ListTripFuelLogsByCompany(ctx context.Context, companyID string) ([]TripFuelLog, error) {
	out := []TripFuelLog{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("purchased_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) ListTripFuelLogsByTrip(ctx context.Context, companyID, tripID string) ([]TripFuelLog, error) {
	out := []TripFuelLog{}
	err := r.db.NewSelect().Model(&out).
		Where("company_id = ?", companyID).
		Where("trip_id = ?", tripID).
		OrderExpr("purchased_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) DeleteTripFuelLog(ctx context.Context, companyID, id string) error {
	_, err := r.db.NewDelete().Model((*TripFuelLog)(nil)).
		Where("company_id = ?", companyID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// ---- Fuel surcharge rules ----

func (r *BunRepository) CreateSurchargeRule(ctx context.Context, rule *FuelSurchargeRule) error {
	rule.IsActive = true
	_, err := r.db.NewInsert().Model(rule).
		Returning("id, is_active, price_variation_percentage, suggested_surcharge_percentage, created_at, updated_at").
		Exec(ctx)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrDuplicateRule
		}
		return err
	}
	return nil
}

func (r *BunRepository) GetSurchargeRuleByID(ctx context.Context, companyID, id string) (*FuelSurchargeRule, error) {
	rule := new(FuelSurchargeRule)
	err := r.db.NewSelect().Model(rule).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *BunRepository) ListSurchargeRulesByCompany(ctx context.Context, companyID string) ([]FuelSurchargeRule, error) {
	out := []FuelSurchargeRule{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) ListActiveSurchargeRulesByCompany(ctx context.Context, companyID string) ([]FuelSurchargeRule, error) {
	out := []FuelSurchargeRule{}
	err := r.db.NewSelect().Model(&out).
		Where("company_id = ?", companyID).
		Where("is_active = ?", true).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) UpdateSurchargeRule(ctx context.Context, rule *FuelSurchargeRule) error {
	_, err := r.db.NewUpdate().Model(rule).
		Set("baseline_price = ?", rule.BaselinePrice).
		Set("threshold_percentage = ?", rule.ThresholdPercentage).
		Set("pass_through_rate = ?", rule.PassThroughRate).
		Set("is_active = ?", rule.IsActive).
		Set("updated_at = now()").
		Where("company_id = ?", rule.CompanyID).
		Where("id = ?", rule.ID).
		Exec(ctx)
	return err
}

func (r *BunRepository) UpdateSurchargeRuleCalculation(ctx context.Context, id string, lastIndexPrice *float64, variationPct, suggestedPct float64) error {
	_, err := r.db.NewUpdate().Model((*FuelSurchargeRule)(nil)).
		Set("last_index_price = ?", lastIndexPrice).
		Set("price_variation_percentage = ?", variationPct).
		Set("suggested_surcharge_percentage = ?", suggestedPct).
		Set("last_calculated_at = now()").
		Set("updated_at = now()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *BunRepository) DeleteSurchargeRule(ctx context.Context, companyID, id string) error {
	_, err := r.db.NewDelete().Model((*FuelSurchargeRule)(nil)).
		Where("company_id = ?", companyID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// ---- Route margin impact ----

// AggregateRouteFuelAndRevenue builds the join + GROUP BY subquery entirely
// through Bun's query builder: an inner SelectQuery (per-trip fuel totals)
// passed as a derived table via TableExpr("(?) AS sub", subq), which Bun
// expands inline — no string-built SQL anywhere.
func (r *BunRepository) AggregateRouteFuelAndRevenue(ctx context.Context, companyID, routeID, fuelType string) (avgFuelCost, avgRevenue float64, tripCount int, err error) {
	subq := r.db.NewSelect().
		ColumnExpr("tfl.trip_id").
		ColumnExpr("SUM(tfl.total_cost) AS trip_total").
		TableExpr("trip_fuel_logs AS tfl").
		Join("JOIN trips AS t ON t.id = tfl.trip_id").
		Where("t.company_id = ?", companyID).
		Where("t.route_id = ?", routeID).
		Where("tfl.fuel_type = ?", fuelType).
		GroupExpr("tfl.trip_id")

	err = r.db.NewSelect().
		ColumnExpr("COALESCE(AVG(sub.trip_total), 0)").
		ColumnExpr("COUNT(*)").
		TableExpr("(?) AS sub", subq).
		Scan(ctx, &avgFuelCost, &tripCount)
	if err != nil {
		return 0, 0, 0, err
	}

	err = r.db.NewSelect().
		ColumnExpr("COALESCE(AVG(agreed_freight_price), 0)").
		TableExpr("trips").
		Where("company_id = ?", companyID).
		Where("route_id = ?", routeID).
		Scan(ctx, &avgRevenue)
	if err != nil {
		return 0, 0, 0, err
	}

	return avgFuelCost, avgRevenue, tripCount, nil
}

func (r *BunRepository) UpsertRouteMarginImpact(ctx context.Context, m *RouteMarginImpact) error {
	_, err := r.db.NewInsert().Model(m).
		On("CONFLICT (company_id, route_id, fuel_type) DO UPDATE").
		Set("baseline_avg_fuel_cost = EXCLUDED.baseline_avg_fuel_cost").
		Set("baseline_avg_revenue = EXCLUDED.baseline_avg_revenue").
		Set("simulated_price_variation_percentage = EXCLUDED.simulated_price_variation_percentage").
		Set("simulated_fuel_cost = EXCLUDED.simulated_fuel_cost").
		Set("cost_increase = EXCLUDED.cost_increase").
		Set("margin_impact_percentage = EXCLUDED.margin_impact_percentage").
		Set("sample_trip_count = EXCLUDED.sample_trip_count").
		Set("calculated_at = now()").
		Returning("id, calculated_at").
		Exec(ctx)
	return err
}

func (r *BunRepository) GetRouteMarginImpact(ctx context.Context, companyID, routeID, fuelType string) (*RouteMarginImpact, error) {
	m := new(RouteMarginImpact)
	err := r.db.NewSelect().Model(m).
		Where("company_id = ?", companyID).
		Where("route_id = ?", routeID).
		Where("fuel_type = ?", fuelType).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *BunRepository) ListRouteMarginImpactsByCompany(ctx context.Context, companyID string) ([]RouteMarginImpact, error) {
	out := []RouteMarginImpact{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("calculated_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}
