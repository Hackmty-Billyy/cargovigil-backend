package company

import (
	"time"

	"github.com/uptrace/bun"
)

// bun:"..." tags map these straight onto the tables from migrations
// 000006/000008 — see domain/fuel/model.go for the convention.

type Company struct {
	bun.BaseModel `bun:"table:companies,alias:c"`

	ID        string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	Name      string    `bun:"name,notnull" json:"name"`
	IsActive  bool      `bun:"is_active,notnull" json:"is_active"`
	CreatedAt time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

type Vehicle struct {
	bun.BaseModel `bun:"table:vehicles,alias:v"`

	ID         string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID  string    `bun:"company_id,notnull" json:"company_id"`
	Type       string    `bun:"type,notnull" json:"type"` // truck | ship | plane
	Identifier string    `bun:"identifier,notnull" json:"identifier"`
	IsActive   bool      `bun:"is_active,notnull" json:"is_active"`
	CreatedAt  time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

type Route struct {
	bun.BaseModel `bun:"table:routes,alias:rt"`

	ID          string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID   string    `bun:"company_id,notnull" json:"company_id"`
	Origin      string    `bun:"origin,notnull" json:"origin"`
	Destination string    `bun:"destination,notnull" json:"destination"`
	DistanceKM  *float64  `bun:"distance_km" json:"distance_km"`
	IsActive    bool      `bun:"is_active,notnull" json:"is_active"`
	CreatedAt   time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

type Client struct {
	bun.BaseModel `bun:"table:clients,alias:cl"`

	ID        string    `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID string    `bun:"company_id,notnull" json:"company_id"`
	Name      string    `bun:"name,notnull" json:"name"`
	TaxID     *string   `bun:"tax_id" json:"tax_id"`
	IsActive  bool      `bun:"is_active,notnull" json:"is_active"`
	CreatedAt time.Time `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}

type Contract struct {
	bun.BaseModel `bun:"table:contracts,alias:ct"`

	ID        string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero" json:"id"`
	CompanyID string     `bun:"company_id,notnull" json:"company_id"`
	ClientID  string     `bun:"client_id,notnull" json:"client_id"`
	Reference string     `bun:"reference,notnull" json:"reference"`
	StartsOn  *time.Time `bun:"starts_on,type:date" json:"starts_on"`
	EndsOn    *time.Time `bun:"ends_on,type:date" json:"ends_on"`
	IsActive  bool       `bun:"is_active,notnull" json:"is_active"`
	CreatedAt time.Time  `bun:"created_at,nullzero,default:now()" json:"created_at"`
	UpdatedAt time.Time  `bun:"updated_at,nullzero,default:now()" json:"updated_at"`
}
