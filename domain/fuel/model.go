package fuel

import (
	"time"

	"github.com/uptrace/bun"
)

// These structs double as Bun models: the `bun:"..."` tags map them
// straight onto the tables from migrations 000014/000021 — no separate
// persistence type. Two of them (FuelIndex, TripFuelLog) live on
// TimescaleDB hypertables whose primary key was widened to include the
// time column (migration 000019, required by create_hypertable), so both
// the id and the time column are tagged `,pk` here to match.
//
// `nullzero,default:...` on id/timestamp columns means: when the Go field
// is left at its zero value, Bun omits that column from the INSERT
// entirely so Postgres's own DEFAULT (gen_random_uuid()/now()) fills it —
// the same thing the old hand-written SQL did with
// `COALESCE($n, now())`/`RETURNING id`.

// FuelIndex is a global (not company-scoped) fuel price reading, matching
// migration 000014. Nothing today integrates a real provider (EIA/Platts/
// etc.), so readings are recorded manually by platform_admin, simulating
// what a scheduled sync would push automatically — same stopgap used for
// treasury's bank feed.
type FuelIndex struct {
	bun.BaseModel `bun:"table:fuel_indexes,alias:fi"`

	ID            string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	FuelType      string    `bun:"fuel_type,notnull" json:"fuel_type"`
	Region        string    `bun:"region,notnull" json:"region"`
	PricePerUnit  float64   `bun:"price_per_unit,notnull" json:"price_per_unit"`
	UnitOfMeasure string    `bun:"unit_of_measure,notnull" json:"unit_of_measure"`
	RecordedAt    time.Time `bun:"recorded_at,pk,nullzero,default:now()" json:"recorded_at"`
	Source        string    `bun:"source,notnull" json:"source"`
}

// FuelPriceWeeklyTrend reads the fuel_price_weekly continuous aggregate
// (materialized view, migration 000019). It is read-only by convention —
// nothing ever inserts/updates through this model, it only backs SELECTs —
// which is the normal way an ORM coexists with a Timescale continuous
// aggregate: the aggregate is refreshed server-side by its own policy, the
// app just queries the resulting view like any other table.
type FuelPriceWeeklyTrend struct {
	bun.BaseModel `bun:"table:fuel_price_weekly,alias:fpw"`

	FuelType    string    `bun:"fuel_type" json:"fuel_type"`
	Region      string    `bun:"region" json:"region"`
	Bucket      time.Time `bun:"bucket" json:"bucket"`
	AvgPrice    float64   `bun:"avg_price" json:"avg_price"`
	MinPrice    float64   `bun:"min_price" json:"min_price"`
	MaxPrice    float64   `bun:"max_price" json:"max_price"`
	SampleCount int64     `bun:"sample_count" json:"sample_count"`
}

// TripFuelLog is an actual fuel purchase tied to a trip, matching migration
// 000014.
type TripFuelLog struct {
	bun.BaseModel `bun:"table:trip_fuel_logs,alias:tfl"`

	ID              string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID       string    `bun:"company_id,notnull" json:"company_id"`
	TripID          string    `bun:"trip_id,notnull" json:"trip_id"`
	FuelType        string    `bun:"fuel_type,notnull" json:"fuel_type"`
	VolumePurchased float64   `bun:"volume_purchased,notnull" json:"volume_purchased"`
	CostPerUnit     float64   `bun:"cost_per_unit,notnull" json:"cost_per_unit"`
	TotalCost       float64   `bun:"total_cost,notnull" json:"total_cost"`
	OdometerOrHours *float64  `bun:"odometer_or_hours" json:"odometer_or_hours"`
	PurchasedAt     time.Time `bun:"purchased_at,pk,nullzero,default:now()" json:"purchased_at"`
}

// FuelSurchargeRule is the recommendation engine's persisted state per
// (company, fuel_type, region): the baseline price the company priced its
// routes/contracts at, how much variation it can absorb before recommending
// a surcharge, and the last computed recommendation.
type FuelSurchargeRule struct {
	bun.BaseModel `bun:"table:fuel_surcharge_rules,alias:fsr"`

	ID                           string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID                    string     `bun:"company_id,notnull" json:"company_id"`
	FuelType                     string     `bun:"fuel_type,notnull" json:"fuel_type"`
	Region                       string     `bun:"region,notnull" json:"region"`
	BaselinePrice                float64    `bun:"baseline_price,notnull" json:"baseline_price"`
	ThresholdPercentage          float64    `bun:"threshold_percentage,notnull" json:"threshold_percentage"`
	PassThroughRate              float64    `bun:"pass_through_rate,notnull" json:"pass_through_rate"`
	LastIndexPrice               *float64   `bun:"last_index_price" json:"last_index_price"`
	PriceVariationPercentage     float64    `bun:"price_variation_percentage,notnull" json:"price_variation_percentage"`
	SuggestedSurchargePercentage float64    `bun:"suggested_surcharge_percentage,notnull" json:"suggested_surcharge_percentage"`
	IsActive                     bool       `bun:"is_active,notnull" json:"is_active"`
	LastCalculatedAt             *time.Time `bun:"last_calculated_at" json:"last_calculated_at"`
	CreatedAt                    time.Time  `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt                    time.Time  `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

// RouteMarginImpact is the latest "what happens to this route's margin if
// fuel moves X%" simulation snapshot for a (company, route, fuel_type).
type RouteMarginImpact struct {
	bun.BaseModel `bun:"table:route_margin_impacts,alias:rmi"`

	ID                                string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID                         string    `bun:"company_id,notnull" json:"company_id"`
	RouteID                           string    `bun:"route_id,notnull" json:"route_id"`
	FuelType                          string    `bun:"fuel_type,notnull" json:"fuel_type"`
	BaselineAvgFuelCost               float64   `bun:"baseline_avg_fuel_cost,notnull" json:"baseline_avg_fuel_cost"`
	BaselineAvgRevenue                float64   `bun:"baseline_avg_revenue,notnull" json:"baseline_avg_revenue"`
	SimulatedPriceVariationPercentage float64   `bun:"simulated_price_variation_percentage,notnull" json:"simulated_price_variation_percentage"`
	SimulatedFuelCost                 float64   `bun:"simulated_fuel_cost,notnull" json:"simulated_fuel_cost"`
	CostIncrease                      float64   `bun:"cost_increase,notnull" json:"cost_increase"`
	MarginImpactPercentage            float64   `bun:"margin_impact_percentage,notnull" json:"margin_impact_percentage"`
	SampleTripCount                   int       `bun:"sample_trip_count,notnull" json:"sample_trip_count"`
	CalculatedAt                      time.Time `bun:"calculated_at,nullzero,default:now()" json:"calculated_at"`
}
