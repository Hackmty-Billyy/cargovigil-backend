package company

import "time"

type Company struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Vehicle struct {
	ID         string    `json:"id"`
	CompanyID  string    `json:"company_id"`
	Type       string    `json:"type"` // truck | ship | plane
	Identifier string    `json:"identifier"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Route struct {
	ID          string    `json:"id"`
	CompanyID   string    `json:"company_id"`
	Origin      string    `json:"origin"`
	Destination string    `json:"destination"`
	DistanceKM  *float64  `json:"distance_km"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Client struct {
	ID        string    `json:"id"`
	CompanyID string    `json:"company_id"`
	Name      string    `json:"name"`
	TaxID     *string   `json:"tax_id"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Contract struct {
	ID        string     `json:"id"`
	CompanyID string     `json:"company_id"`
	ClientID  string     `json:"client_id"`
	Reference string     `json:"reference"`
	StartsOn  *time.Time `json:"starts_on"`
	EndsOn    *time.Time `json:"ends_on"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}
