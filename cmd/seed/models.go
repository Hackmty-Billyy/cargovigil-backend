package main

import (
	"time"

	"github.com/uptrace/bun"
)

// These are Bun models scoped to the seeder only, for tables that belong to
// modules without a Go domain package yet (Módulos 2/4/5 — trips itself,
// trip_frictions, route_risk_profiles, client_payment_behavior,
// pod_documents, trip_profitability). They exist so the seeder never drops
// to a raw SQL string, same convention as domain/*/model.go: `bun:"..."`
// tags mapped straight onto the tables from migrations 000011/000013/
// 000015/000016. trip_fuel_logs and bank_accounts reuse the real domain
// models (fuel.TripFuelLog, treasury.BankAccount) instead of duplicating
// them here.

type Trip struct {
	bun.BaseModel `bun:"table:trips,alias:t"`

	ID                    string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	CompanyID             string     `bun:"company_id,notnull"`
	VehicleID             string     `bun:"vehicle_id,notnull"`
	RouteID               string     `bun:"route_id,notnull"`
	ClientID              string     `bun:"client_id,notnull"`
	ContractID            *string    `bun:"contract_id"`
	TrackingCode          string     `bun:"tracking_code,notnull"`
	CargoType             string     `bun:"cargo_type,notnull"`
	CargoWeightTons       float64    `bun:"cargo_weight_tons,notnull"`
	Status                string     `bun:"status,notnull"`
	DepartureDate         time.Time  `bun:"departure_date,notnull"`
	EstimatedArrivalDate  time.Time  `bun:"estimated_arrival_date,notnull"`
	ActualArrivalDate     *time.Time `bun:"actual_arrival_date"`
	AgreedFreightPrice    float64    `bun:"agreed_freight_price,notnull"`
	Currency              string     `bun:"currency,notnull"`
	FuelSurchargeAmount   float64    `bun:"fuel_surcharge_amount,notnull"`
	ContingencyBudget     float64    `bun:"contingency_budget,notnull"`
	CreatedAt             time.Time  `bun:"created_at,nullzero,default:now()"`
	UpdatedAt             time.Time  `bun:"updated_at,nullzero,default:now()"`
}

type TripFriction struct {
	bun.BaseModel `bun:"table:trip_frictions,alias:tf"`

	ID            string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	CompanyID     string     `bun:"company_id,notnull"`
	TripID        string     `bun:"trip_id,notnull"`
	EventType     string     `bun:"event_type,notnull"`
	LocationName  string     `bun:"location_name"`
	StartedAt     time.Time  `bun:"started_at,pk,notnull"`
	EndedAt       *time.Time `bun:"ended_at"`
	DurationHours float64    `bun:"duration_hours,notnull"`
	CostImpact    float64    `bun:"cost_impact,notnull"`
	Notes         string     `bun:"notes"`
	CreatedAt     time.Time  `bun:"created_at,nullzero,default:now()"`
}

type RouteRiskProfile struct {
	bun.BaseModel `bun:"table:route_risk_profiles,alias:rrp"`

	ID                               string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	CompanyID                        string    `bun:"company_id,notnull"`
	RouteID                          string    `bun:"route_id,notnull"`
	HistoricalRiskScore              float64   `bun:"historical_risk_score,notnull"`
	AvgDelayHours                    float64   `bun:"avg_delay_hours,notnull"`
	SuggestedContingencyPercentage   float64   `bun:"suggested_contingency_percentage,notnull"`
	IncidentCount                    int       `bun:"incident_count,notnull"`
	LastCalculatedAt                 time.Time `bun:"last_calculated_at,nullzero,default:now()"`
}

type ClientPaymentBehavior struct {
	bun.BaseModel `bun:"table:client_payment_behavior,alias:cpb"`

	ID                       string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	CompanyID                string    `bun:"company_id,notnull"`
	ClientID                 string    `bun:"client_id,notnull"`
	AveragePODApprovalDays   float64   `bun:"average_pod_approval_days,notnull"`
	AveragePaymentDelayDays  float64   `bun:"average_payment_delay_days,notnull"`
	DisputeRatePercentage    float64   `bun:"dispute_rate_percentage,notnull"`
	LastCalculatedAt         time.Time `bun:"last_calculated_at,nullzero,default:now()"`
}

type PODDocument struct {
	bun.BaseModel `bun:"table:pod_documents,alias:pod"`

	ID            string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	CompanyID     string     `bun:"company_id,notnull"`
	TripID        string     `bun:"trip_id,notnull"`
	ClientID      string     `bun:"client_id,notnull"`
	DocumentURL   string     `bun:"document_url"`
	Status        string     `bun:"status,notnull"`
	DisputeReason *string    `bun:"dispute_reason"`
	DeliveryDate  *time.Time `bun:"delivery_date"`
	SignatureDate *time.Time `bun:"signature_date"`
	DaysToSign    int        `bun:"days_to_sign,notnull"`
	CreatedAt     time.Time  `bun:"created_at,nullzero,default:now()"`
	UpdatedAt     time.Time  `bun:"updated_at,nullzero,default:now()"`
}

type TripProfitability struct {
	bun.BaseModel `bun:"table:trip_profitability,alias:tp"`

	ID                     string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	CompanyID              string    `bun:"company_id,notnull"`
	TripID                 string    `bun:"trip_id,notnull"`
	GrossRevenue           float64   `bun:"gross_revenue,notnull"`
	FuelCost               float64   `bun:"fuel_cost,notnull"`
	TollAndPortCost        float64   `bun:"toll_and_port_cost,notnull"`
	DriverCost             float64   `bun:"driver_cost,notnull"`
	FrictionCost           float64   `bun:"friction_cost,notnull"`
	MaintenanceAllocation  float64   `bun:"maintenance_allocation,notnull"`
	TotalCost              float64   `bun:"total_cost,notnull"`
	NetProfit              float64   `bun:"net_profit,notnull"`
	ProfitMarginPercentage float64   `bun:"profit_margin_percentage,notnull"`
	ProfitPerKM            float64   `bun:"profit_per_km,notnull"`
	CreatedAt              time.Time `bun:"created_at,nullzero,default:now()"`
	UpdatedAt              time.Time `bun:"updated_at,nullzero,default:now()"`
}
