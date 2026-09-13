package routecost

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// As in domain/treasury, method names are suffixed per entity instead of the
// bare Create/Update used in domain/auth: one PostgresRepository implements
// every interface below on purpose, because the money-moving operations of
// this module (allocating a cushion, settling a friction) have to touch
// trips, trip_contingency_funds and expenses inside a single transaction.
//
// This package writes `expenses` rows directly even though treasury owns that
// table. The alternative — calling treasury's service — cannot give us the
// atomicity we need here (the reserve and the fund must move together or not
// at all). Treasury remains the owner of the expense *lifecycle*: paying one
// still goes through /treasury/expenses/:id/pay.

type TripFilter struct {
	Status  string
	RouteID string
}

type TripRepository interface {
	CreateTrip(ctx context.Context, t *Trip) error
	GetTripByID(ctx context.Context, companyID, id string) (*Trip, error)
	ListTripsByCompany(ctx context.Context, companyID string, filter TripFilter) ([]Trip, error)
	UpdateTrip(ctx context.Context, t *Trip) error
	UpdateTripStatus(ctx context.Context, companyID, id, status string, actualArrival *time.Time) (*Trip, error)
	DeleteTrip(ctx context.Context, companyID, id string) error
	ListRouteIDsByCompany(ctx context.Context, companyID string) ([]string, error)
}

// FrictionSettlement is the treasury side of a friction: the real payable it
// generates, already converted to TreasuryCurrency by the service. It is nil
// when the event cost nothing out of pocket (pure idle time).
type FrictionSettlement struct {
	ExpenseCategory    string
	ExpenseDescription string
	ExpenseAmount      float64
	ExpenseDueDate     time.Time
}

type FrictionRepository interface {
	CreateFriction(ctx context.Context, f *Friction, settlement *FrictionSettlement) error
	GetFrictionByID(ctx context.Context, companyID, id string) (*Friction, error)
	ListFrictionsByTrip(ctx context.Context, companyID, tripID string) ([]Friction, error)
	ListFrictionsByCompany(ctx context.Context, companyID string, onlyOpen bool) ([]Friction, error)
	CloseFriction(ctx context.Context, f *Friction, settlement *FrictionSettlement) error
}

// RiskStats is the raw aggregate the scoring formula runs on.
type RiskStats struct {
	TripCount      int
	DelayedTrips   int
	AvgDelayHours  float64
	IncidentCount  int
	TotalIdleHours float64
}

type RiskProfileRepository interface {
	GetRiskProfile(ctx context.Context, companyID, routeID string) (*RouteRiskProfile, error)
	ListRiskProfilesByCompany(ctx context.Context, companyID string) ([]RouteRiskProfile, error)
	UpsertRiskProfile(ctx context.Context, p *RouteRiskProfile) error
	RiskStatsForRoute(ctx context.Context, companyID, routeID string, since time.Time) (RiskStats, error)
}

// ReserveExpense is the pending expense that mirrors a cushion into the cash
// flow forecast of Module 1.
type ReserveExpense struct {
	Description string
	Amount      float64
	DueDate     time.Time
}

type ContingencyFundRepository interface {
	GetFundByTrip(ctx context.Context, companyID, tripID string) (*ContingencyFund, error)
	ListFundsByCompany(ctx context.Context, companyID string) ([]ContingencyFund, error)
	AllocateFund(ctx context.Context, f *ContingencyFund, reserve ReserveExpense, mirrorBudget float64) (*ContingencyFund, error)
	ReleaseFund(ctx context.Context, companyID, tripID string) (*ContingencyFund, error)
}

type CostSettingsRepository interface {
	GetCostSettings(ctx context.Context, companyID string) (*CostSettings, error)
	UpsertCostSettings(ctx context.Context, s *CostSettings) error
}

type PostgresRepository struct{ db *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// rowQuerier is satisfied by both the pool and a transaction, so the same
// helper can run inside or outside a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// pgErrorAs maps the Postgres failures this module can legitimately hit on
// user input: a malformed uuid or a dangling foreign key (garbage ids from a
// client) and the unique tracking code per company.
func pgErrorAs(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "22P02", "23503":
			return ErrInvalidReference
		case "23505":
			return ErrDuplicateTracking
		}
	}
	return err
}

// ---- Trips ----

const tripColumns = `id, company_id, vehicle_id, route_id, client_id, contract_id, tracking_code, cargo_type,
	cargo_weight_tons, status, departure_date, estimated_arrival_date, actual_arrival_date,
	agreed_freight_price, currency, fuel_surcharge_amount, contingency_budget, created_at, updated_at`

func scanTrip(row pgx.Row) (*Trip, error) {
	t := &Trip{}
	err := row.Scan(&t.ID, &t.CompanyID, &t.VehicleID, &t.RouteID, &t.ClientID, &t.ContractID, &t.TrackingCode,
		&t.CargoType, &t.CargoWeightTons, &t.Status, &t.DepartureDate, &t.EstimatedArrivalDate, &t.ActualArrivalDate,
		&t.AgreedFreightPrice, &t.Currency, &t.FuelSurchargeAmount, &t.ContingencyBudget, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, pgErrorAs(err)
	}
	return t, nil
}

// validateTripRefs is what keeps tenancy honest: a trip may only point at
// vehicles/routes/clients/contracts of its own company. Without it an admin
// could attach another tenant's fleet to their trip just by guessing a uuid.
func (r *PostgresRepository) validateTripRefs(ctx context.Context, q rowQuerier, t *Trip) error {
	var okVehicle, okRoute, okClient, okContract bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM vehicles  WHERE id = $1 AND company_id = $5),
		        EXISTS (SELECT 1 FROM routes    WHERE id = $2 AND company_id = $5),
		        EXISTS (SELECT 1 FROM clients   WHERE id = $3 AND company_id = $5),
		        ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM contracts WHERE id = $4 AND company_id = $5))`,
		t.VehicleID, t.RouteID, t.ClientID, t.ContractID, t.CompanyID,
	).Scan(&okVehicle, &okRoute, &okClient, &okContract)
	if err != nil {
		return pgErrorAs(err)
	}
	if !okVehicle || !okRoute || !okClient || !okContract {
		return ErrInvalidReference
	}
	return nil
}

func (r *PostgresRepository) CreateTrip(ctx context.Context, t *Trip) error {
	if err := r.validateTripRefs(ctx, r.db, t); err != nil {
		return err
	}
	err := r.db.QueryRow(ctx,
		`INSERT INTO trips (company_id, vehicle_id, route_id, client_id, contract_id, tracking_code, cargo_type,
		                    cargo_weight_tons, status, departure_date, estimated_arrival_date, actual_arrival_date,
		                    agreed_freight_price, currency, fuel_surcharge_amount, contingency_budget)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 RETURNING id, status, contingency_budget, created_at, updated_at`,
		t.CompanyID, t.VehicleID, t.RouteID, t.ClientID, t.ContractID, t.TrackingCode, t.CargoType,
		t.CargoWeightTons, t.Status, t.DepartureDate, t.EstimatedArrivalDate, t.ActualArrivalDate,
		t.AgreedFreightPrice, t.Currency, t.FuelSurchargeAmount, t.ContingencyBudget,
	).Scan(&t.ID, &t.Status, &t.ContingencyBudget, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return pgErrorAs(err)
	}
	return nil
}

func (r *PostgresRepository) GetTripByID(ctx context.Context, companyID, id string) (*Trip, error) {
	return scanTrip(r.db.QueryRow(ctx, `SELECT `+tripColumns+` FROM trips WHERE company_id = $1 AND id = $2`, companyID, id))
}

func (r *PostgresRepository) ListTripsByCompany(ctx context.Context, companyID string, filter TripFilter) ([]Trip, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+tripColumns+` FROM trips
		 WHERE company_id = $1
		   AND ($2 = '' OR status = $2)
		   AND ($3 = '' OR route_id = $3::uuid)
		 ORDER BY departure_date DESC`,
		companyID, filter.Status, filter.RouteID)
	if err != nil {
		return nil, pgErrorAs(err)
	}
	defer rows.Close()

	list := []Trip{}
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *t)
	}
	return list, rows.Err()
}

func (r *PostgresRepository) UpdateTrip(ctx context.Context, t *Trip) error {
	if err := r.validateTripRefs(ctx, r.db, t); err != nil {
		return err
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE trips SET vehicle_id=$1, route_id=$2, client_id=$3, contract_id=$4, tracking_code=$5, cargo_type=$6,
		                  cargo_weight_tons=$7, departure_date=$8, estimated_arrival_date=$9, actual_arrival_date=$10,
		                  agreed_freight_price=$11, currency=$12, fuel_surcharge_amount=$13, updated_at=now()
		 WHERE company_id=$14 AND id=$15`,
		t.VehicleID, t.RouteID, t.ClientID, t.ContractID, t.TrackingCode, t.CargoType, t.CargoWeightTons,
		t.DepartureDate, t.EstimatedArrivalDate, t.ActualArrivalDate, t.AgreedFreightPrice, t.Currency,
		t.FuelSurchargeAmount, t.CompanyID, t.ID)
	if err != nil {
		return pgErrorAs(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateTripStatus(ctx context.Context, companyID, id, status string, actualArrival *time.Time) (*Trip, error) {
	return scanTrip(r.db.QueryRow(ctx,
		`UPDATE trips
		    SET status = $3,
		        actual_arrival_date = COALESCE($4, actual_arrival_date),
		        updated_at = now()
		  WHERE company_id = $1 AND id = $2
		  RETURNING `+tripColumns,
		companyID, id, status, actualArrival))
}

func (r *PostgresRepository) DeleteTrip(ctx context.Context, companyID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM trips WHERE company_id = $1 AND id = $2`, companyID, id)
	if err != nil {
		return pgErrorAs(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListRouteIDsByCompany(ctx context.Context, companyID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM routes WHERE company_id = $1 AND is_active = true`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---- Frictions ----

const frictionColumns = `id, company_id, trip_id, event_type, location_name, started_at, ended_at,
	duration_hours, cost_impact, opportunity_cost, notes, created_at`

func scanFriction(row pgx.Row) (*Friction, error) {
	f := &Friction{}
	err := row.Scan(&f.ID, &f.CompanyID, &f.TripID, &f.EventType, &f.LocationName, &f.StartedAt, &f.EndedAt,
		&f.DurationHours, &f.CostImpact, &f.OpportunityCost, &f.Notes, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, pgErrorAs(err)
	}
	return f, nil
}

func (r *PostgresRepository) CreateFriction(ctx context.Context, f *Friction, settlement *FrictionSettlement) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`INSERT INTO trip_frictions (company_id, trip_id, event_type, location_name, started_at, ended_at,
		                             duration_hours, cost_impact, opportunity_cost, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, created_at`,
		f.CompanyID, f.TripID, f.EventType, f.LocationName, f.StartedAt, f.EndedAt,
		f.DurationHours, f.CostImpact, f.OpportunityCost, f.Notes,
	).Scan(&f.ID, &f.CreatedAt)
	if err != nil {
		return pgErrorAs(err)
	}

	if err := applySettlement(ctx, tx, f.CompanyID, f.TripID, settlement); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) GetFrictionByID(ctx context.Context, companyID, id string) (*Friction, error) {
	return scanFriction(r.db.QueryRow(ctx,
		`SELECT `+frictionColumns+` FROM trip_frictions WHERE company_id = $1 AND id = $2`, companyID, id))
}

func (r *PostgresRepository) ListFrictionsByTrip(ctx context.Context, companyID, tripID string) ([]Friction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+frictionColumns+` FROM trip_frictions
		 WHERE company_id = $1 AND trip_id = $2 ORDER BY started_at DESC`, companyID, tripID)
	if err != nil {
		return nil, pgErrorAs(err)
	}
	return collectFrictions(rows)
}

func (r *PostgresRepository) ListFrictionsByCompany(ctx context.Context, companyID string, onlyOpen bool) ([]Friction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+frictionColumns+` FROM trip_frictions
		 WHERE company_id = $1 AND ($2 = false OR ended_at IS NULL)
		 ORDER BY started_at DESC`, companyID, onlyOpen)
	if err != nil {
		return nil, pgErrorAs(err)
	}
	return collectFrictions(rows)
}

func collectFrictions(rows pgx.Rows) ([]Friction, error) {
	defer rows.Close()

	list := []Friction{}
	for rows.Next() {
		f, err := scanFriction(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *f)
	}
	return list, rows.Err()
}

func (r *PostgresRepository) CloseFriction(ctx context.Context, f *Friction, settlement *FrictionSettlement) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	updated, err := scanFriction(tx.QueryRow(ctx,
		`UPDATE trip_frictions
		    SET ended_at = $3, duration_hours = $4, cost_impact = $5, opportunity_cost = $6, notes = COALESCE($7, notes)
		  WHERE company_id = $1 AND id = $2
		  RETURNING `+frictionColumns,
		f.CompanyID, f.ID, f.EndedAt, f.DurationHours, f.CostImpact, f.OpportunityCost, f.Notes))
	if err != nil {
		return err
	}

	if err := applySettlement(ctx, tx, updated.CompanyID, updated.TripID, settlement); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	*f = *updated
	return nil
}

// applySettlement is the anti-double-counting rule: the real payable a
// friction generates is charged *against* the trip's cushion first (shrinking
// the reserve expense by the same amount) and only the part that exceeds the
// cushion is new money leaving the forecast.
func applySettlement(ctx context.Context, tx pgx.Tx, companyID, tripID string, s *FrictionSettlement) error {
	if s == nil || s.ExpenseAmount <= 0 {
		return nil
	}

	fund, err := scanFund(tx.QueryRow(ctx,
		`SELECT `+fundColumns+` FROM trip_contingency_funds
		  WHERE company_id = $1 AND trip_id = $2 FOR UPDATE`, companyID, tripID))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}

	if fund != nil && fund.Status != "released" {
		consume := fund.RemainingAmount()
		if consume > s.ExpenseAmount {
			consume = s.ExpenseAmount
		}
		if consume > 0 {
			newConsumed := round2(fund.ConsumedAmount + consume)
			status := "partially_consumed"
			if newConsumed >= fund.AllocatedAmount-0.005 {
				status = "exhausted"
			}
			if _, err := tx.Exec(ctx,
				`UPDATE trip_contingency_funds SET consumed_amount = $2, status = $3, updated_at = now() WHERE id = $1`,
				fund.ID, newConsumed, status); err != nil {
				return err
			}
			if fund.ReserveExpenseID != nil {
				remaining := round2(fund.AllocatedAmount - newConsumed - fund.ReleasedAmount)
				if remaining < 0 {
					remaining = 0
				}
				if err := shrinkReserveExpense(ctx, tx, *fund.ReserveExpenseID, remaining); err != nil {
					return err
				}
			}
		}
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO expenses (company_id, trip_id, category, description, amount, due_date, status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pending')`,
		companyID, tripID, s.ExpenseCategory, s.ExpenseDescription, s.ExpenseAmount, s.ExpenseDueDate)
	return pgErrorAs(err)
}

// shrinkReserveExpense only touches a reserve that is still pending: if
// finance already paid it, the money is gone and rewriting it would corrupt
// the ledger.
func shrinkReserveExpense(ctx context.Context, tx pgx.Tx, expenseID string, remaining float64) error {
	status := "pending"
	if remaining <= 0 {
		status = "cancelled"
	}
	_, err := tx.Exec(ctx,
		`UPDATE expenses SET amount = $2, status = $3, updated_at = now()
		  WHERE id = $1 AND status = 'pending'`, expenseID, remaining, status)
	return err
}

// ---- Route risk profiles ----

const riskColumns = `id, company_id, route_id, historical_risk_score, avg_delay_hours,
	suggested_contingency_percentage, incident_count, last_calculated_at`

func scanRiskProfile(row pgx.Row) (*RouteRiskProfile, error) {
	p := &RouteRiskProfile{}
	err := row.Scan(&p.ID, &p.CompanyID, &p.RouteID, &p.HistoricalRiskScore, &p.AvgDelayHours,
		&p.SuggestedContingencyPercentage, &p.IncidentCount, &p.LastCalculatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, pgErrorAs(err)
	}
	return p, nil
}

func (r *PostgresRepository) GetRiskProfile(ctx context.Context, companyID, routeID string) (*RouteRiskProfile, error) {
	return scanRiskProfile(r.db.QueryRow(ctx,
		`SELECT `+riskColumns+` FROM route_risk_profiles WHERE company_id = $1 AND route_id = $2`, companyID, routeID))
}

func (r *PostgresRepository) ListRiskProfilesByCompany(ctx context.Context, companyID string) ([]RouteRiskProfile, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+riskColumns+` FROM route_risk_profiles WHERE company_id = $1 ORDER BY historical_risk_score DESC`, companyID)
	if err != nil {
		return nil, pgErrorAs(err)
	}
	defer rows.Close()

	list := []RouteRiskProfile{}
	for rows.Next() {
		p, err := scanRiskProfile(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

func (r *PostgresRepository) UpsertRiskProfile(ctx context.Context, p *RouteRiskProfile) error {
	err := r.db.QueryRow(ctx,
		`INSERT INTO route_risk_profiles (company_id, route_id, historical_risk_score, avg_delay_hours,
		                                  suggested_contingency_percentage, incident_count, last_calculated_at)
		 VALUES ($1,$2,$3,$4,$5,$6, now())
		 ON CONFLICT (company_id, route_id) DO UPDATE
		   SET historical_risk_score = EXCLUDED.historical_risk_score,
		       avg_delay_hours = EXCLUDED.avg_delay_hours,
		       suggested_contingency_percentage = EXCLUDED.suggested_contingency_percentage,
		       incident_count = EXCLUDED.incident_count,
		       last_calculated_at = now()
		 RETURNING id, last_calculated_at`,
		p.CompanyID, p.RouteID, p.HistoricalRiskScore, p.AvgDelayHours,
		p.SuggestedContingencyPercentage, p.IncidentCount,
	).Scan(&p.ID, &p.LastCalculatedAt)
	return pgErrorAs(err)
}

func (r *PostgresRepository) RiskStatsForRoute(ctx context.Context, companyID, routeID string, since time.Time) (RiskStats, error) {
	var s RiskStats
	err := r.db.QueryRow(ctx,
		`WITH t AS (
		    SELECT id, status, estimated_arrival_date, actual_arrival_date
		      FROM trips
		     WHERE company_id = $1 AND route_id = $2::uuid AND departure_date >= $3
		 ), delays AS (
		    SELECT GREATEST(0, EXTRACT(EPOCH FROM (
		               CASE WHEN status = 'completed' AND actual_arrival_date IS NOT NULL THEN actual_arrival_date
		                    WHEN status = 'delayed' THEN now()
		               END - estimated_arrival_date)) / 3600.0) AS delay_hours
		      FROM t
		     WHERE (status = 'completed' AND actual_arrival_date IS NOT NULL) OR status = 'delayed'
		 )
		 SELECT (SELECT count(*) FROM t),
		        (SELECT count(*) FROM delays WHERE delay_hours > 0),
		        COALESCE((SELECT avg(delay_hours) FROM delays), 0),
		        (SELECT count(*) FROM trip_frictions f JOIN t ON t.id = f.trip_id),
		        COALESCE((SELECT sum(f.duration_hours) FROM trip_frictions f JOIN t ON t.id = f.trip_id), 0)`,
		companyID, routeID, since,
	).Scan(&s.TripCount, &s.DelayedTrips, &s.AvgDelayHours, &s.IncidentCount, &s.TotalIdleHours)
	if err != nil {
		return RiskStats{}, pgErrorAs(err)
	}
	return s, nil
}

// ---- Contingency funds ----

const fundColumns = `id, company_id, trip_id, route_risk_score, applied_percentage, base_amount, base_currency,
	fx_rate, reserve_currency, allocated_amount, consumed_amount, released_amount, status, reserve_expense_id,
	calculated_at, created_at, updated_at`

func scanFund(row pgx.Row) (*ContingencyFund, error) {
	f := &ContingencyFund{}
	err := row.Scan(&f.ID, &f.CompanyID, &f.TripID, &f.RouteRiskScore, &f.AppliedPercentage, &f.BaseAmount,
		&f.BaseCurrency, &f.FXRate, &f.ReserveCurrency, &f.AllocatedAmount, &f.ConsumedAmount, &f.ReleasedAmount,
		&f.Status, &f.ReserveExpenseID, &f.CalculatedAt, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, pgErrorAs(err)
	}
	return f, nil
}

func (r *PostgresRepository) GetFundByTrip(ctx context.Context, companyID, tripID string) (*ContingencyFund, error) {
	return scanFund(r.db.QueryRow(ctx,
		`SELECT `+fundColumns+` FROM trip_contingency_funds WHERE company_id = $1 AND trip_id = $2`, companyID, tripID))
}

func (r *PostgresRepository) ListFundsByCompany(ctx context.Context, companyID string) ([]ContingencyFund, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+fundColumns+` FROM trip_contingency_funds WHERE company_id = $1 ORDER BY calculated_at DESC`, companyID)
	if err != nil {
		return nil, pgErrorAs(err)
	}
	defer rows.Close()

	list := []ContingencyFund{}
	for rows.Next() {
		f, err := scanFund(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *f)
	}
	return list, rows.Err()
}

// AllocateFund creates or re-sizes the cushion of a trip and keeps its mirror
// row in expenses in sync, all in one transaction. Re-allocating never drops
// the cushion below what has already been consumed, and a released fund is
// never resurrected.
func (r *PostgresRepository) AllocateFund(ctx context.Context, f *ContingencyFund, reserve ReserveExpense, mirrorBudget float64) (*ContingencyFund, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	existing, err := scanFund(tx.QueryRow(ctx,
		`SELECT `+fundColumns+` FROM trip_contingency_funds WHERE company_id = $1 AND trip_id = $2 FOR UPDATE`,
		f.CompanyID, f.TripID))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if existing != nil && existing.Status == "released" {
		return nil, ErrFundReleased
	}

	var saved *ContingencyFund
	if existing == nil {
		saved, err = scanFund(tx.QueryRow(ctx,
			`INSERT INTO trip_contingency_funds (company_id, trip_id, route_risk_score, applied_percentage,
			        base_amount, base_currency, fx_rate, reserve_currency, allocated_amount, status, calculated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'allocated', now())
			 RETURNING `+fundColumns,
			f.CompanyID, f.TripID, f.RouteRiskScore, f.AppliedPercentage, f.BaseAmount, f.BaseCurrency,
			f.FXRate, f.ReserveCurrency, f.AllocatedAmount))
	} else {
		allocated := f.AllocatedAmount
		if allocated < existing.ConsumedAmount {
			allocated = existing.ConsumedAmount
		}
		status := "allocated"
		if existing.ConsumedAmount > 0 {
			status = "partially_consumed"
			if existing.ConsumedAmount >= allocated-0.005 {
				status = "exhausted"
			}
		}
		saved, err = scanFund(tx.QueryRow(ctx,
			`UPDATE trip_contingency_funds
			    SET route_risk_score = $2, applied_percentage = $3, base_amount = $4, base_currency = $5,
			        fx_rate = $6, reserve_currency = $7, allocated_amount = $8, status = $9,
			        calculated_at = now(), updated_at = now()
			  WHERE id = $1
			  RETURNING `+fundColumns,
			existing.ID, f.RouteRiskScore, f.AppliedPercentage, f.BaseAmount, f.BaseCurrency,
			f.FXRate, f.ReserveCurrency, allocated, status))
	}
	if err != nil {
		return nil, err
	}

	remaining := saved.RemainingAmount()
	if saved.ReserveExpenseID == nil {
		var expenseID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO expenses (company_id, trip_id, category, description, amount, due_date, status)
			 VALUES ($1, $2, 'contingency_reserve', $3, $4, $5, 'pending')
			 RETURNING id`,
			saved.CompanyID, saved.TripID, reserve.Description, remaining, reserve.DueDate,
		).Scan(&expenseID); err != nil {
			return nil, pgErrorAs(err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE trip_contingency_funds SET reserve_expense_id = $2, updated_at = now() WHERE id = $1`,
			saved.ID, expenseID); err != nil {
			return nil, err
		}
		saved.ReserveExpenseID = &expenseID
	} else if _, err := tx.Exec(ctx,
		`UPDATE expenses SET amount = $2, due_date = $3, description = $4, updated_at = now()
		  WHERE id = $1 AND status = 'pending'`,
		*saved.ReserveExpenseID, remaining, reserve.DueDate, reserve.Description); err != nil {
		return nil, err
	}

	// trips.contingency_budget stays the denormalised mirror (in the trip's
	// own currency) that modules 3-5 already read.
	if _, err := tx.Exec(ctx,
		`UPDATE trips SET contingency_budget = $3, updated_at = now() WHERE company_id = $1 AND id = $2`,
		saved.CompanyID, saved.TripID, mirrorBudget); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return saved, nil
}

// ReleaseFund frees whatever is left of the cushion and drops it out of the
// forecast. It is idempotent: releasing an already released fund is a no-op,
// which matters because completing a trip triggers it automatically.
func (r *PostgresRepository) ReleaseFund(ctx context.Context, companyID, tripID string) (*ContingencyFund, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	fund, err := scanFund(tx.QueryRow(ctx,
		`SELECT `+fundColumns+` FROM trip_contingency_funds WHERE company_id = $1 AND trip_id = $2 FOR UPDATE`,
		companyID, tripID))
	if err != nil {
		return nil, err
	}
	if fund.Status == "released" {
		return fund, nil
	}

	released := round2(fund.ReleasedAmount + fund.RemainingAmount())
	updated, err := scanFund(tx.QueryRow(ctx,
		`UPDATE trip_contingency_funds SET released_amount = $2, status = 'released', updated_at = now()
		  WHERE id = $1 RETURNING `+fundColumns, fund.ID, released))
	if err != nil {
		return nil, err
	}

	if fund.ReserveExpenseID != nil {
		if err := shrinkReserveExpense(ctx, tx, *fund.ReserveExpenseID, 0); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return updated, nil
}

// ---- Cost settings ----

func (r *PostgresRepository) GetCostSettings(ctx context.Context, companyID string) (*CostSettings, error) {
	s := &CostSettings{}
	err := r.db.QueryRow(ctx,
		`SELECT company_id, idle_hourly_rate, currency, updated_at FROM company_cost_settings WHERE company_id = $1`,
		companyID).Scan(&s.CompanyID, &s.IdleHourlyRate, &s.Currency, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, pgErrorAs(err)
	}
	return s, nil
}

func (r *PostgresRepository) UpsertCostSettings(ctx context.Context, s *CostSettings) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO company_cost_settings (company_id, idle_hourly_rate, currency, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (company_id) DO UPDATE
		   SET idle_hourly_rate = EXCLUDED.idle_hourly_rate,
		       currency = EXCLUDED.currency,
		       updated_at = now()
		 RETURNING updated_at`,
		s.CompanyID, s.IdleHourlyRate, s.Currency).Scan(&s.UpdatedAt)
}
