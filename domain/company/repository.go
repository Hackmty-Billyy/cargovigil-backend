package company

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

const pgForeignKeyViolation = "23503"

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

// deleteOrSoftDelete tries a hard delete; if it's blocked by a referencing
// row elsewhere (23503 — e.g. a vehicle already used on a trip), it falls
// back to flipping is_active instead of failing the request outright.
func deleteOrSoftDelete(ctx context.Context, db *bun.DB, table, companyID, id string) error {
	_, err := db.NewDelete().Table(table).Where("company_id = ?", companyID).Where("id = ?", id).Exec(ctx)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			_, err = db.NewUpdate().Table(table).
				Set("is_active = false").
				Set("updated_at = now()").
				Where("company_id = ?", companyID).
				Where("id = ?", id).
				Exec(ctx)
			return err
		}
	}
	return err
}

// ---- Bun implementations ----

type BunCompanyRepository struct{ db *bun.DB }

func NewBunCompanyRepository(db *bun.DB) *BunCompanyRepository {
	return &BunCompanyRepository{db: db}
}

func (r *BunCompanyRepository) Create(ctx context.Context, c *Company) error {
	_, err := r.db.NewInsert().Model(c).Column("name").Returning("id, is_active, created_at, updated_at").Exec(ctx)
	return err
}

func (r *BunCompanyRepository) GetByID(ctx context.Context, id string) (*Company, error) {
	c := new(Company)
	err := r.db.NewSelect().Model(c).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *BunCompanyRepository) ListAll(ctx context.Context) ([]Company, error) {
	out := []Company{}
	err := r.db.NewSelect().Model(&out).OrderExpr("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunCompanyRepository) SetActive(ctx context.Context, id string, isActive bool) error {
	_, err := r.db.NewUpdate().Model((*Company)(nil)).
		Set("is_active = ?", isActive).
		Set("updated_at = now()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

type BunVehicleRepository struct{ db *bun.DB }

func NewBunVehicleRepository(db *bun.DB) *BunVehicleRepository {
	return &BunVehicleRepository{db: db}
}

func (r *BunVehicleRepository) Create(ctx context.Context, v *Vehicle) error {
	v.IsActive = true
	_, err := r.db.NewInsert().Model(v).
		Column("company_id", "type", "identifier", "is_active").
		Returning("id, is_active, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunVehicleRepository) GetByID(ctx context.Context, companyID, id string) (*Vehicle, error) {
	v := new(Vehicle)
	err := r.db.NewSelect().Model(v).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (r *BunVehicleRepository) ListByCompany(ctx context.Context, companyID string) ([]Vehicle, error) {
	out := []Vehicle{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunVehicleRepository) Update(ctx context.Context, v *Vehicle) error {
	_, err := r.db.NewUpdate().Model(v).
		Set("type = ?", v.Type).
		Set("identifier = ?", v.Identifier).
		Set("is_active = ?", v.IsActive).
		Set("updated_at = now()").
		Where("company_id = ?", v.CompanyID).
		Where("id = ?", v.ID).
		Exec(ctx)
	return err
}

func (r *BunVehicleRepository) Delete(ctx context.Context, companyID, id string) error {
	return deleteOrSoftDelete(ctx, r.db, "vehicles", companyID, id)
}

type BunRouteRepository struct{ db *bun.DB }

func NewBunRouteRepository(db *bun.DB) *BunRouteRepository {
	return &BunRouteRepository{db: db}
}

func (r *BunRouteRepository) Create(ctx context.Context, rt *Route) error {
	rt.IsActive = true
	_, err := r.db.NewInsert().Model(rt).
		Column("company_id", "origin", "destination", "distance_km", "is_active").
		Returning("id, is_active, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunRouteRepository) GetByID(ctx context.Context, companyID, id string) (*Route, error) {
	rt := new(Route)
	err := r.db.NewSelect().Model(rt).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *BunRouteRepository) ListByCompany(ctx context.Context, companyID string) ([]Route, error) {
	out := []Route{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunRouteRepository) Update(ctx context.Context, rt *Route) error {
	_, err := r.db.NewUpdate().Model(rt).
		Set("origin = ?", rt.Origin).
		Set("destination = ?", rt.Destination).
		Set("distance_km = ?", rt.DistanceKM).
		Set("is_active = ?", rt.IsActive).
		Set("updated_at = now()").
		Where("company_id = ?", rt.CompanyID).
		Where("id = ?", rt.ID).
		Exec(ctx)
	return err
}

func (r *BunRouteRepository) Delete(ctx context.Context, companyID, id string) error {
	return deleteOrSoftDelete(ctx, r.db, "routes", companyID, id)
}

type BunClientRepository struct{ db *bun.DB }

func NewBunClientRepository(db *bun.DB) *BunClientRepository {
	return &BunClientRepository{db: db}
}

func (r *BunClientRepository) Create(ctx context.Context, cl *Client) error {
	cl.IsActive = true
	_, err := r.db.NewInsert().Model(cl).
		Column("company_id", "name", "tax_id", "is_active").
		Returning("id, is_active, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunClientRepository) GetByID(ctx context.Context, companyID, id string) (*Client, error) {
	cl := new(Client)
	err := r.db.NewSelect().Model(cl).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return cl, nil
}

func (r *BunClientRepository) ListByCompany(ctx context.Context, companyID string) ([]Client, error) {
	out := []Client{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunClientRepository) Update(ctx context.Context, cl *Client) error {
	_, err := r.db.NewUpdate().Model(cl).
		Set("name = ?", cl.Name).
		Set("tax_id = ?", cl.TaxID).
		Set("is_active = ?", cl.IsActive).
		Set("updated_at = now()").
		Where("company_id = ?", cl.CompanyID).
		Where("id = ?", cl.ID).
		Exec(ctx)
	return err
}

func (r *BunClientRepository) Delete(ctx context.Context, companyID, id string) error {
	return deleteOrSoftDelete(ctx, r.db, "clients", companyID, id)
}

type BunContractRepository struct{ db *bun.DB }

func NewBunContractRepository(db *bun.DB) *BunContractRepository {
	return &BunContractRepository{db: db}
}

func (r *BunContractRepository) Create(ctx context.Context, ct *Contract) error {
	ct.IsActive = true
	_, err := r.db.NewInsert().Model(ct).
		Column("company_id", "client_id", "reference", "starts_on", "ends_on", "is_active").
		Returning("id, is_active, created_at, updated_at").
		Exec(ctx)
	return err
}

func (r *BunContractRepository) GetByID(ctx context.Context, companyID, id string) (*Contract, error) {
	ct := new(Contract)
	err := r.db.NewSelect().Model(ct).Where("company_id = ?", companyID).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return ct, nil
}

func (r *BunContractRepository) ListByCompany(ctx context.Context, companyID string) ([]Contract, error) {
	out := []Contract{}
	err := r.db.NewSelect().Model(&out).Where("company_id = ?", companyID).OrderExpr("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BunContractRepository) Update(ctx context.Context, ct *Contract) error {
	_, err := r.db.NewUpdate().Model(ct).
		Set("client_id = ?", ct.ClientID).
		Set("reference = ?", ct.Reference).
		Set("starts_on = ?", ct.StartsOn).
		Set("ends_on = ?", ct.EndsOn).
		Set("is_active = ?", ct.IsActive).
		Set("updated_at = now()").
		Where("company_id = ?", ct.CompanyID).
		Where("id = ?", ct.ID).
		Exec(ctx)
	return err
}

func (r *BunContractRepository) Delete(ctx context.Context, companyID, id string) error {
	return deleteOrSoftDelete(ctx, r.db, "contracts", companyID, id)
}
