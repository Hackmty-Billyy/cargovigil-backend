package logistics

import (
	"time"

	"github.com/uptrace/bun"
)

// This package is the read/orchestration layer the live radar dashboard talks
// to. It owns no business rules of its own: trips are created through
// domain/routecost (which is what assigns the contingency cushion), fuel
// prices come from domain/fuel, and route risk from route_risk_profiles. What
// it adds is the shape the map needs — one trip row already joined with its
// vehicle, route and client, plus the tracking columns from migration 000022.

// Trip is the enriched trip the radar renders. The columns up to
// LastSimulatedStep are real trips columns; everything after is filled by the
// joins in the repository and is read-only (scanonly), never written back.
type Trip struct {
	bun.BaseModel `bun:"table:trips,alias:t"`

	ID                   string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID            string     `bun:"company_id,notnull" json:"company_id"`
	VehicleID            string     `bun:"vehicle_id,notnull" json:"vehicle_id"`
	RouteID              string     `bun:"route_id,notnull" json:"route_id"`
	ClientID             string     `bun:"client_id,notnull" json:"client_id"`
	ContractID           *string    `bun:"contract_id" json:"contract_id"`
	TrackingCode         string     `bun:"tracking_code,notnull" json:"tracking_code"`
	CargoType            string     `bun:"cargo_type,notnull" json:"cargo_type"`
	CargoWeightTons      float64    `bun:"cargo_weight_tons,notnull" json:"cargo_weight_tons"`
	Status               string     `bun:"status,notnull" json:"status"`
	DepartureDate        time.Time  `bun:"departure_date,notnull" json:"departure_date"`
	EstimatedArrivalDate time.Time  `bun:"estimated_arrival_date,notnull" json:"estimated_arrival_date"`
	ActualArrivalDate    *time.Time `bun:"actual_arrival_date" json:"actual_arrival_date"`
	AgreedFreightPrice   float64    `bun:"agreed_freight_price,notnull" json:"agreed_freight_price"`
	Currency             string     `bun:"currency,notnull" json:"currency"`
	FuelSurchargeAmount  float64    `bun:"fuel_surcharge_amount,notnull" json:"fuel_surcharge_amount"`
	ContingencyBudget    float64    `bun:"contingency_budget,notnull" json:"contingency_budget"`

	// Live tracking (migration 000022), fed by the simulation endpoint.
	CurrentLat         *float64 `bun:"current_lat" json:"current_lat"`
	CurrentLng         *float64 `bun:"current_lng" json:"current_lng"`
	ProgressPercentage float64  `bun:"progress_percentage,notnull" json:"progress_percentage"`
	IsStuck            bool     `bun:"is_stuck,notnull" json:"is_stuck"`
	StuckReason        *string  `bun:"stuck_reason" json:"stuck_reason"`
	EstimatedFuelCost  float64  `bun:"estimated_fuel_cost,notnull" json:"estimated_fuel_cost"`
	EstimatedLossRisk  float64  `bun:"estimated_loss_risk,notnull" json:"estimated_loss_risk"`
	LastSimulatedStep  int      `bun:"last_simulated_step,notnull" json:"last_simulated_step"`

	// Step at which the current incident began (migration 000023), so the
	// friction it opened can be closed with the simulated hours it actually
	// lasted instead of the seconds of wall clock between two button clicks.
	StuckSinceStep int `bun:"stuck_since_step,notnull" json:"-"`

	CreatedAt time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`

	// Joined, read-only.
	VehicleIdentifier string   `bun:"vehicle_identifier,scanonly" json:"vehicle_identifier"`
	VehicleType       string   `bun:"vehicle_type,scanonly" json:"vehicle_type"`
	RouteOrigin       string   `bun:"route_origin,scanonly" json:"route_origin"`
	RouteDestination  string   `bun:"route_destination,scanonly" json:"route_destination"`
	RouteDistanceKM   *float64 `bun:"route_distance_km,scanonly" json:"route_distance_km"`
	OriginLat         *float64 `bun:"origin_lat,scanonly" json:"origin_lat"`
	OriginLng         *float64 `bun:"origin_lng,scanonly" json:"origin_lng"`
	DestLat           *float64 `bun:"dest_lat,scanonly" json:"dest_lat"`
	DestLng           *float64 `bun:"dest_lng,scanonly" json:"dest_lng"`
	ClientName        string   `bun:"client_name,scanonly" json:"client_name"`

	// Route risk, joined for the simulation's loss estimate. Not part of the
	// radar payload — the risk profile has its own endpoint in /routecost.
	RiskScore     float64 `bun:"risk_score,scanonly" json:"-"`
	RiskAvgDelayH float64 `bun:"risk_avg_delay_hours,scanonly" json:"-"`
}

// ProjectionRequest is what the pre-trip scheduler asks before committing a
// trip: given this asset, route, load and price, is it worth running?
type ProjectionRequest struct {
	VehicleType     string  `json:"vehicle_type"`
	RouteID         string  `json:"route_id"`
	DistanceKM      float64 `json:"distance_km"`
	CargoWeightTons float64 `json:"cargo_weight_tons"`
	AgreedPrice     float64 `json:"agreed_price"`
	Currency        string  `json:"currency"`
}

// ProjectionResponse mixes the three modules that have something to say about
// a trip before it starts: fuel (module 3), route risk and the contingency
// cushion (module 2). Every money field is expressed in ProjectionRequest's
// currency.
type ProjectionResponse struct {
	DistanceKM            float64 `json:"distance_km"`
	FuelType              string  `json:"fuel_type"`
	CurrentFuelPrice      float64 `json:"current_fuel_price"`
	FuelUnit              string  `json:"fuel_unit"`
	FuelSource            string  `json:"fuel_source"`
	EstimatedFuelQuantity float64 `json:"estimated_fuel_quantity"`
	EstimatedFuelCost     float64 `json:"estimated_fuel_cost"`
	SuggestedContingency  float64 `json:"suggested_contingency"`
	RiskScore             float64 `json:"risk_score"`
	AverageDelayHours     float64 `json:"average_delay_hours"`
	PotentialLossRisk     float64 `json:"potential_loss_risk"`
	ProjectedNetMargin    float64 `json:"projected_net_margin"`
	ProjectedMarginPct    float64 `json:"projected_margin_pct"`
	Recommendation        string  `json:"recommendation"`
}

// SimulationResult is the payload of the "advance time" button.
type SimulationResult struct {
	Message string `json:"message"`
	Trips   []Trip `json:"trips"`
}
