package company

import (
	"context"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
)

var allowedTeammateRoles = map[string]bool{
	"admin":      true,
	"operations": true,
	"finance":    true,
}

type Service struct {
	companies CompanyRepository
	vehicles  VehicleRepository
	routes    RouteRepository
	clients   ClientRepository
	contracts ContractRepository
	auth      *auth.Service
}

func NewService(companies CompanyRepository, vehicles VehicleRepository, routes RouteRepository,
	clients ClientRepository, contracts ContractRepository, authService *auth.Service) *Service {
	return &Service{
		companies: companies,
		vehicles:  vehicles,
		routes:    routes,
		clients:   clients,
		contracts: contracts,
		auth:      authService,
	}
}

type SignupResult struct {
	Company *Company
	User    *auth.User
}

// Signup is the self-service entry point: it creates a brand new Company and
// its first user as admin. Role is never taken from the caller here — the
// founder of a company is always admin.
func (s *Service) Signup(ctx context.Context, companyName, email, password string) (*SignupResult, error) {
	c := &Company{Name: companyName}
	if err := s.companies.Create(ctx, c); err != nil {
		return nil, err
	}

	u, err := s.auth.Register(ctx, c.ID, email, password, "admin")
	if err != nil {
		return nil, err
	}

	return &SignupResult{Company: c, User: u}, nil
}

// InviteTeammate adds a new user to an existing company. Only an admin of
// that company should be allowed to call this (enforced by the handler's
// role middleware, not here).
func (s *Service) InviteTeammate(ctx context.Context, companyID, email, password, role string) (*auth.User, error) {
	if !allowedTeammateRoles[role] {
		return nil, ErrInvalidRole
	}
	return s.auth.Register(ctx, companyID, email, password, role)
}

func (s *Service) ListCompanies(ctx context.Context) ([]Company, error) {
	return s.companies.ListAll(ctx)
}

func (s *Service) SetCompanyActive(ctx context.Context, id string, isActive bool) error {
	return s.companies.SetActive(ctx, id, isActive)
}

func (s *Service) CreateVehicle(ctx context.Context, v *Vehicle) error {
	return s.vehicles.Create(ctx, v)
}

func (s *Service) ListVehicles(ctx context.Context, companyID string) ([]Vehicle, error) {
	return s.vehicles.ListByCompany(ctx, companyID)
}

func (s *Service) UpdateVehicle(ctx context.Context, v *Vehicle) error {
	return s.vehicles.Update(ctx, v)
}

func (s *Service) DeleteVehicle(ctx context.Context, companyID, id string) error {
	return s.vehicles.Delete(ctx, companyID, id)
}

func (s *Service) CreateRoute(ctx context.Context, r *Route) error {
	return s.routes.Create(ctx, r)
}

func (s *Service) ListRoutes(ctx context.Context, companyID string) ([]Route, error) {
	return s.routes.ListByCompany(ctx, companyID)
}

func (s *Service) UpdateRoute(ctx context.Context, r *Route) error {
	return s.routes.Update(ctx, r)
}

func (s *Service) DeleteRoute(ctx context.Context, companyID, id string) error {
	return s.routes.Delete(ctx, companyID, id)
}

func (s *Service) CreateClient(ctx context.Context, cl *Client) error {
	return s.clients.Create(ctx, cl)
}

func (s *Service) ListClients(ctx context.Context, companyID string) ([]Client, error) {
	return s.clients.ListByCompany(ctx, companyID)
}

func (s *Service) UpdateClient(ctx context.Context, cl *Client) error {
	return s.clients.Update(ctx, cl)
}

func (s *Service) DeleteClient(ctx context.Context, companyID, id string) error {
	return s.clients.Delete(ctx, companyID, id)
}

func (s *Service) CreateContract(ctx context.Context, ct *Contract) error {
	return s.contracts.Create(ctx, ct)
}

func (s *Service) ListContracts(ctx context.Context, companyID string) ([]Contract, error) {
	return s.contracts.ListByCompany(ctx, companyID)
}

func (s *Service) UpdateContract(ctx context.Context, ct *Contract) error {
	return s.contracts.Update(ctx, ct)
}

func (s *Service) DeleteContract(ctx context.Context, companyID, id string) error {
	return s.contracts.Delete(ctx, companyID, id)
}
