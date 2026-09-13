package routecost

import "time"

// Trip is the central operational entity of the platform: modules 1, 3, 4 and
// 5 all hang off trip_id. It lives here because its lifecycle (completing a
// trip releases the contingency cushion and re-scores the route) is Module 2
// logic.
type Trip struct {
	ID                   string     `json:"id"`
	CompanyID            string     `json:"company_id"`
	VehicleID            string     `json:"vehicle_id"`
	RouteID              string     `json:"route_id"`
	ClientID             string     `json:"client_id"`
	ContractID           *string    `json:"contract_id"`
	TrackingCode         string     `json:"tracking_code"`
	CargoType            string     `json:"cargo_type"`
	CargoWeightTons      float64    `json:"cargo_weight_tons"`
	Status               string     `json:"status"`
	DepartureDate        time.Time  `json:"departure_date"`
	EstimatedArrivalDate time.Time  `json:"estimated_arrival_date"`
	ActualArrivalDate    *time.Time `json:"actual_arrival_date"`
	AgreedFreightPrice   float64    `json:"agreed_freight_price"`
	Currency             string     `json:"currency"`
	FuelSurchargeAmount  float64    `json:"fuel_surcharge_amount"`
	ContingencyBudget    float64    `json:"contingency_budget"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// Friction is the DowntimeEvent of the module spec (table trip_frictions):
// port demurrage, customs holds, warehouse detention and so on.
//
// CostImpact is money actually owed to a third party and is expressed in the
// trip's currency. OpportunityCost is what the idle hours cost in revenue the
// asset could not produce, same currency, computed when the event is closed.
type Friction struct {
	ID              string     `json:"id"`
	CompanyID       string     `json:"company_id"`
	TripID          string     `json:"trip_id"`
	EventType       string     `json:"event_type"`
	LocationName    *string    `json:"location_name"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	DurationHours   float64    `json:"duration_hours"`
	CostImpact      float64    `json:"cost_impact"`
	OpportunityCost float64    `json:"opportunity_cost"`
	Notes           *string    `json:"notes"`
	CreatedAt       time.Time  `json:"created_at"`

	// Joined for the listings: a downtime inbox that shows only trip_id is
	// unreadable, and making the client resolve every uuid against the
	// catalogue would be one request per row.
	TripTrackingCode string `json:"trip_tracking_code,omitempty"`
	TripStatus       string `json:"trip_status,omitempty"`
	RouteOrigin      string `json:"route_origin,omitempty"`
	RouteDestination string `json:"route_destination,omitempty"`
}

// RouteRiskProfile is the rolling score of a route, recalculated from the
// trips and frictions of the last RiskWindowDays days.
type RouteRiskProfile struct {
	ID                             string    `json:"id"`
	CompanyID                      string    `json:"company_id"`
	RouteID                        string    `json:"route_id"`
	HistoricalRiskScore            float64   `json:"historical_risk_score"`
	AvgDelayHours                  float64   `json:"avg_delay_hours"`
	SuggestedContingencyPercentage float64   `json:"suggested_contingency_percentage"`
	IncidentCount                  int       `json:"incident_count"`
	LastCalculatedAt               time.Time `json:"last_calculated_at"`

	// Joined: the score means nothing without knowing which road it scores.
	RouteOrigin      string   `json:"route_origin,omitempty"`
	RouteDestination string   `json:"route_destination,omitempty"`
	RouteDistanceKM  *float64 `json:"route_distance_km,omitempty"`
}

// ContingencyFund is the liquidity cushion assigned to one trip.
//
// BaseAmount/BaseCurrency are the trip's freight price at calculation time;
// AllocatedAmount and everything downstream are expressed in ReserveCurrency
// (the treasury currency), converted with FXRate. Keeping both sides recorded
// is what makes the number auditable after the fact.
type ContingencyFund struct {
	ID                string    `json:"id"`
	CompanyID         string    `json:"company_id"`
	TripID            string    `json:"trip_id"`
	RouteRiskScore    float64   `json:"route_risk_score"`
	AppliedPercentage float64   `json:"applied_percentage"`
	BaseAmount        float64   `json:"base_amount"`
	BaseCurrency      string    `json:"base_currency"`
	FXRate            float64   `json:"fx_rate"`
	ReserveCurrency   string    `json:"reserve_currency"`
	AllocatedAmount   float64   `json:"allocated_amount"`
	ConsumedAmount    float64   `json:"consumed_amount"`
	ReleasedAmount    float64   `json:"released_amount"`
	Status            string    `json:"status"`
	ReserveExpenseID  *string   `json:"reserve_expense_id"`
	CalculatedAt      time.Time `json:"calculated_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// Joined, same reason as Friction's.
	TripTrackingCode string `json:"trip_tracking_code,omitempty"`
	TripStatus       string `json:"trip_status,omitempty"`
	RouteOrigin      string `json:"route_origin,omitempty"`
	RouteDestination string `json:"route_destination,omitempty"`
}

// RemainingAmount is what is still reserved and unspent.
func (f *ContingencyFund) RemainingAmount() float64 {
	remaining := f.AllocatedAmount - f.ConsumedAmount - f.ReleasedAmount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// CostSettings is the optional per-company override for the idle hourly rate.
// A nil IdleHourlyRate means "derive it from each trip's own revenue rate".
type CostSettings struct {
	CompanyID      string    `json:"company_id"`
	IdleHourlyRate *float64  `json:"idle_hourly_rate"`
	Currency       string    `json:"currency"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TripImpact is the read model that answers "what did the friction on this
// trip actually cost me", in the trip's currency.
type TripImpact struct {
	TripID              string           `json:"trip_id"`
	Currency            string           `json:"currency"`
	FrictionCount       int              `json:"friction_count"`
	OpenFrictionCount   int              `json:"open_friction_count"`
	TotalIdleHours      float64          `json:"total_idle_hours"`
	DirectCost          float64          `json:"direct_cost"`
	OpportunityCost     float64          `json:"opportunity_cost"`
	TotalFrictionImpact float64          `json:"total_friction_impact"`
	IdleHourlyRate      float64          `json:"idle_hourly_rate"`
	Fund                *ContingencyFund `json:"contingency_fund"`
}
