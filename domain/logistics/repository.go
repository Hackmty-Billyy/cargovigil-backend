package logistics

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

type Repository interface {
	ListTrips(ctx context.Context, companyID string) ([]Trip, error)
	GetTrip(ctx context.Context, companyID, id string) (*Trip, error)
	ListSimulatableTrips(ctx context.Context, companyID, tripID string) ([]Trip, error)
	UpdateTracking(ctx context.Context, t *Trip) error
}

// BunRepository reads through Bun like the rest of the domains. Every query
// here is a join: the radar needs the vehicle's plate, the route's endpoints
// and the client's name in the same row as the trip, and doing that in the
// database beats four round trips per render.
type BunRepository struct{ db *bun.DB }

func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// simulatableStatuses are the states a trip can still move from. A completed
// or cancelled trip is frozen: the simulation skips it.
var simulatableStatuses = []string{"scheduled", "in_transit", "delayed"}

func (r *BunRepository) baseSelect(trips *[]Trip, companyID string) *bun.SelectQuery {
	return r.db.NewSelect().
		Model(trips).
		ColumnExpr("t.*").
		ColumnExpr("COALESCE(v.identifier, '') AS vehicle_identifier").
		ColumnExpr("COALESCE(v.type, 'truck') AS vehicle_type").
		ColumnExpr("COALESCE(rt.origin, '') AS route_origin").
		ColumnExpr("COALESCE(rt.destination, '') AS route_destination").
		ColumnExpr("rt.distance_km AS route_distance_km").
		ColumnExpr("rt.origin_lat AS origin_lat").
		ColumnExpr("rt.origin_lng AS origin_lng").
		ColumnExpr("rt.dest_lat AS dest_lat").
		ColumnExpr("rt.dest_lng AS dest_lng").
		ColumnExpr("COALESCE(cl.name, '') AS client_name").
		ColumnExpr("COALESCE(rrp.historical_risk_score, 1.00) AS risk_score").
		ColumnExpr("COALESCE(rrp.avg_delay_hours, 0.00) AS risk_avg_delay_hours").
		Join("LEFT JOIN vehicles AS v ON v.id = t.vehicle_id").
		Join("LEFT JOIN routes AS rt ON rt.id = t.route_id").
		Join("LEFT JOIN clients AS cl ON cl.id = t.client_id").
		Join("LEFT JOIN route_risk_profiles AS rrp ON rrp.route_id = t.route_id AND rrp.company_id = t.company_id").
		Where("t.company_id = ?", companyID)
}

func (r *BunRepository) ListTrips(ctx context.Context, companyID string) ([]Trip, error) {
	trips := []Trip{}
	err := r.baseSelect(&trips, companyID).
		OrderExpr("CASE WHEN t.status IN ('in_transit', 'delayed') THEN 0 ELSE 1 END").
		Order("t.departure_date DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return trips, nil
}

func (r *BunRepository) GetTrip(ctx context.Context, companyID, id string) (*Trip, error) {
	trips := []Trip{}
	err := r.baseSelect(&trips, companyID).Where("t.id = ?", id).Limit(1).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if len(trips) == 0 {
		return nil, ErrNotFound
	}
	return &trips[0], nil
}

func (r *BunRepository) ListSimulatableTrips(ctx context.Context, companyID, tripID string) ([]Trip, error) {
	trips := []Trip{}
	q := r.baseSelect(&trips, companyID).
		Where("t.status IN (?)", bun.In(simulatableStatuses)).
		Order("t.departure_date ASC")
	if tripID != "" {
		q = q.Where("t.id = ?", tripID)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return trips, nil
}

// UpdateTracking writes back only the tracking columns. The commercial fields
// of a trip (price, dates, cargo) are never touched here — those belong to
// /routecost.
func (r *BunRepository) UpdateTracking(ctx context.Context, t *Trip) error {
	t.UpdatedAt = time.Now()
	res, err := r.db.NewUpdate().
		Model(t).
		Column("status", "actual_arrival_date", "current_lat", "current_lng", "progress_percentage",
			"is_stuck", "stuck_reason", "estimated_fuel_cost", "estimated_loss_risk",
			"last_simulated_step", "stuck_since_step", "updated_at").
		Where("id = ? AND company_id = ?", t.ID, t.CompanyID).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
