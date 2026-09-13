package company

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CompanyRepository interface {
	Create(ctx context.Context, c *Company) error
	GetByID(ctx context.Context, id string) (*Company, error)
	ListAll(ctx context.Context) ([]Company, error)
	SetActive(ctx context.Context, id string, isActive bool) error
}

type VehicleRepository interface {
	Create(ctx context.Context, v *Vehicle) error
	GetByID(ctx context.Context, companyID, id string) (*Vehicle, error)
	ListByCompany(ctx context.Context, companyID string) ([]Vehicle, error)
	Update(ctx context.Context, v *Vehicle) error
	Delete(ctx context.Context, companyID, id string) error
}

type RouteRepository interface {
	Create(ctx context.Context, r *Route) error
	GetByID(ctx context.Context, companyID, id string) (*Route, error)
	ListByCompany(ctx context.Context, companyID string) ([]Route, error)
	Update(ctx context.Context, r *Route) error
	Delete(ctx context.Context, companyID, id string) error
}

type ClientRepository interface {
	Create(ctx context.Context, cl *Client) error
	GetByID(ctx context.Context, companyID, id string) (*Client, error)
	ListByCompany(ctx context.Context, companyID string) ([]Client, error)
	Update(ctx context.Context, cl *Client) error
	Delete(ctx context.Context, companyID, id string) error
}

type ContractRepository interface {
	Create(ctx context.Context, ct *Contract) error
	GetByID(ctx context.Context, companyID, id string) (*Contract, error)
	ListByCompany(ctx context.Context, companyID string) ([]Contract, error)
	Update(ctx context.Context, ct *Contract) error
	Delete(ctx context.Context, companyID, id string) error
}

// ---- Postgres implementations ----

type PostgresCompanyRepository struct{ db *pgxpool.Pool }

func NewPostgresCompanyRepository(db *pgxpool.Pool) *PostgresCompanyRepository {
	return &PostgresCompanyRepository{db: db}
}

func (r *PostgresCompanyRepository) Create(ctx context.Context, c *Company) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO companies (name) VALUES ($1) RETURNING id, is_active, created_at, updated_at`,
		c.Name,
	).Scan(&c.ID, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
}

func (r *PostgresCompanyRepository) GetByID(ctx context.Context, id string) (*Company, error) {
	c := &Company{}
	err := r.db.QueryRow(ctx,
		`SELECT id, name, is_active, created_at, updated_at FROM companies WHERE id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *PostgresCompanyRepository) ListAll(ctx context.Context) ([]Company, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, is_active, created_at, updated_at FROM companies ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Company{}
	for rows.Next() {
		var c Company
		if err := rows.Scan(&c.ID, &c.Name, &c.IsActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresCompanyRepository) SetActive(ctx context.Context, id string, isActive bool) error {
	_, err := r.db.Exec(ctx, `UPDATE companies SET is_active = $1, updated_at = now() WHERE id = $2`, isActive, id)
	return err
}

type PostgresVehicleRepository struct{ db *pgxpool.Pool }

func NewPostgresVehicleRepository(db *pgxpool.Pool) *PostgresVehicleRepository {
	return &PostgresVehicleRepository{db: db}
}

func (r *PostgresVehicleRepository) Create(ctx context.Context, v *Vehicle) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO vehicles (company_id, type, identifier, is_active)
		 VALUES ($1, $2, $3, true)
		 RETURNING id, is_active, created_at, updated_at`,
		v.CompanyID, v.Type, v.Identifier,
	).Scan(&v.ID, &v.IsActive, &v.CreatedAt, &v.UpdatedAt)
}

func (r *PostgresVehicleRepository) GetByID(ctx context.Context, companyID, id string) (*Vehicle, error) {
	v := &Vehicle{}
	err := r.db.QueryRow(ctx,
		`SELECT id, company_id, type, identifier, is_active, created_at, updated_at
		 FROM vehicles WHERE company_id = $1 AND id = $2`, companyID, id,
	).Scan(&v.ID, &v.CompanyID, &v.Type, &v.Identifier, &v.IsActive, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (r *PostgresVehicleRepository) ListByCompany(ctx context.Context, companyID string) ([]Vehicle, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, company_id, type, identifier, is_active, created_at, updated_at
		 FROM vehicles WHERE company_id = $1 ORDER BY created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Vehicle{}
	for rows.Next() {
		var v Vehicle
		if err := rows.Scan(&v.ID, &v.CompanyID, &v.Type, &v.Identifier, &v.IsActive, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *PostgresVehicleRepository) Update(ctx context.Context, v *Vehicle) error {
	_, err := r.db.Exec(ctx,
		`UPDATE vehicles SET type = $1, identifier = $2, is_active = $3, updated_at = now()
		 WHERE company_id = $4 AND id = $5`,
		v.Type, v.Identifier, v.IsActive, v.CompanyID, v.ID)
	return err
}

func (r *PostgresVehicleRepository) Delete(ctx context.Context, companyID, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM vehicles WHERE company_id = $1 AND id = $2`, companyID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			_, err = r.db.Exec(ctx, `UPDATE vehicles SET is_active = false, updated_at = now() WHERE company_id = $1 AND id = $2`, companyID, id)
			return err
		}
	}
	return err
}

type PostgresRouteRepository struct{ db *pgxpool.Pool }

func NewPostgresRouteRepository(db *pgxpool.Pool) *PostgresRouteRepository {
	return &PostgresRouteRepository{db: db}
}

func (r *PostgresRouteRepository) Create(ctx context.Context, rt *Route) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO routes (company_id, origin, destination, distance_km, is_active)
		 VALUES ($1, $2, $3, $4, true)
		 RETURNING id, is_active, created_at, updated_at`,
		rt.CompanyID, rt.Origin, rt.Destination, rt.DistanceKM,
	).Scan(&rt.ID, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt)
}

func (r *PostgresRouteRepository) GetByID(ctx context.Context, companyID, id string) (*Route, error) {
	rt := &Route{}
	err := r.db.QueryRow(ctx,
		`SELECT id, company_id, origin, destination, distance_km, is_active, created_at, updated_at
		 FROM routes WHERE company_id = $1 AND id = $2`, companyID, id,
	).Scan(&rt.ID, &rt.CompanyID, &rt.Origin, &rt.Destination, &rt.DistanceKM, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *PostgresRouteRepository) ListByCompany(ctx context.Context, companyID string) ([]Route, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, company_id, origin, destination, distance_km, is_active, created_at, updated_at
		 FROM routes WHERE company_id = $1 ORDER BY created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Route{}
	for rows.Next() {
		var rt Route
		if err := rows.Scan(&rt.ID, &rt.CompanyID, &rt.Origin, &rt.Destination, &rt.DistanceKM, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func (r *PostgresRouteRepository) Update(ctx context.Context, rt *Route) error {
	_, err := r.db.Exec(ctx,
		`UPDATE routes SET origin = $1, destination = $2, distance_km = $3, is_active = $4, updated_at = now()
		 WHERE company_id = $5 AND id = $6`,
		rt.Origin, rt.Destination, rt.DistanceKM, rt.IsActive, rt.CompanyID, rt.ID)
	return err
}

func (r *PostgresRouteRepository) Delete(ctx context.Context, companyID, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM routes WHERE company_id = $1 AND id = $2`, companyID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			_, err = r.db.Exec(ctx, `UPDATE routes SET is_active = false, updated_at = now() WHERE company_id = $1 AND id = $2`, companyID, id)
			return err
		}
	}
	return err
}

type PostgresClientRepository struct{ db *pgxpool.Pool }

func NewPostgresClientRepository(db *pgxpool.Pool) *PostgresClientRepository {
	return &PostgresClientRepository{db: db}
}

func (r *PostgresClientRepository) Create(ctx context.Context, cl *Client) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO clients (company_id, name, tax_id, is_active)
		 VALUES ($1, $2, $3, true)
		 RETURNING id, is_active, created_at, updated_at`,
		cl.CompanyID, cl.Name, cl.TaxID,
	).Scan(&cl.ID, &cl.IsActive, &cl.CreatedAt, &cl.UpdatedAt)
}

func (r *PostgresClientRepository) GetByID(ctx context.Context, companyID, id string) (*Client, error) {
	cl := &Client{}
	err := r.db.QueryRow(ctx,
		`SELECT id, company_id, name, tax_id, is_active, created_at, updated_at
		 FROM clients WHERE company_id = $1 AND id = $2`, companyID, id,
	).Scan(&cl.ID, &cl.CompanyID, &cl.Name, &cl.TaxID, &cl.IsActive, &cl.CreatedAt, &cl.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return cl, nil
}

func (r *PostgresClientRepository) ListByCompany(ctx context.Context, companyID string) ([]Client, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, company_id, name, tax_id, is_active, created_at, updated_at
		 FROM clients WHERE company_id = $1 ORDER BY created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Client{}
	for rows.Next() {
		var cl Client
		if err := rows.Scan(&cl.ID, &cl.CompanyID, &cl.Name, &cl.TaxID, &cl.IsActive, &cl.CreatedAt, &cl.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, cl)
	}
	return out, rows.Err()
}

func (r *PostgresClientRepository) Update(ctx context.Context, cl *Client) error {
	_, err := r.db.Exec(ctx,
		`UPDATE clients SET name = $1, tax_id = $2, is_active = $3, updated_at = now()
		 WHERE company_id = $4 AND id = $5`,
		cl.Name, cl.TaxID, cl.IsActive, cl.CompanyID, cl.ID)
	return err
}

func (r *PostgresClientRepository) Delete(ctx context.Context, companyID, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM clients WHERE company_id = $1 AND id = $2`, companyID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			_, err = r.db.Exec(ctx, `UPDATE clients SET is_active = false, updated_at = now() WHERE company_id = $1 AND id = $2`, companyID, id)
			return err
		}
	}
	return err
}

type PostgresContractRepository struct{ db *pgxpool.Pool }

func NewPostgresContractRepository(db *pgxpool.Pool) *PostgresContractRepository {
	return &PostgresContractRepository{db: db}
}

func (r *PostgresContractRepository) Create(ctx context.Context, ct *Contract) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO contracts (company_id, client_id, reference, starts_on, ends_on, is_active)
		 VALUES ($1, $2, $3, $4, $5, true)
		 RETURNING id, is_active, created_at, updated_at`,
		ct.CompanyID, ct.ClientID, ct.Reference, ct.StartsOn, ct.EndsOn,
	).Scan(&ct.ID, &ct.IsActive, &ct.CreatedAt, &ct.UpdatedAt)
}

func (r *PostgresContractRepository) GetByID(ctx context.Context, companyID, id string) (*Contract, error) {
	ct := &Contract{}
	err := r.db.QueryRow(ctx,
		`SELECT id, company_id, client_id, reference, starts_on, ends_on, is_active, created_at, updated_at
		 FROM contracts WHERE company_id = $1 AND id = $2`, companyID, id,
	).Scan(&ct.ID, &ct.CompanyID, &ct.ClientID, &ct.Reference, &ct.StartsOn, &ct.EndsOn, &ct.IsActive, &ct.CreatedAt, &ct.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return ct, nil
}

func (r *PostgresContractRepository) ListByCompany(ctx context.Context, companyID string) ([]Contract, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, company_id, client_id, reference, starts_on, ends_on, is_active, created_at, updated_at
		 FROM contracts WHERE company_id = $1 ORDER BY created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Contract{}
	for rows.Next() {
		var ct Contract
		if err := rows.Scan(&ct.ID, &ct.CompanyID, &ct.ClientID, &ct.Reference, &ct.StartsOn, &ct.EndsOn, &ct.IsActive, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ct)
	}
	return out, rows.Err()
}

func (r *PostgresContractRepository) Update(ctx context.Context, ct *Contract) error {
	_, err := r.db.Exec(ctx,
		`UPDATE contracts SET client_id = $1, reference = $2, starts_on = $3, ends_on = $4, is_active = $5, updated_at = now()
		 WHERE company_id = $6 AND id = $7`,
		ct.ClientID, ct.Reference, ct.StartsOn, ct.EndsOn, ct.IsActive, ct.CompanyID, ct.ID)
	return err
}

func (r *PostgresContractRepository) Delete(ctx context.Context, companyID, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM contracts WHERE company_id = $1 AND id = $2`, companyID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			_, err = r.db.Exec(ctx, `UPDATE contracts SET is_active = false, updated_at = now() WHERE company_id = $1 AND id = $2`, companyID, id)
			return err
		}
	}
	return err
}
