package auth

// Fixed role ids, matching the deterministic insert order of migration
// 000001_create_roles (admin, operations, finance).
const (
	RoleAdminID         int16 = 1
	RoleOperationsID    int16 = 2
	RoleFinanceID       int16 = 3
	RolePlatformAdminID int16 = 4
)
