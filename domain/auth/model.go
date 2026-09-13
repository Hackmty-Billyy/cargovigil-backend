package auth

import (
	"time"

	"github.com/uptrace/bun"
)

// These structs double as Bun models (see domain/fuel/model.go for the same
// convention): `bun:"..."` tags map them straight onto the tables from
// migrations 000001-000004/000007/000009. Schema stays owned by
// golang-migrate; Bun only ever queries/writes rows here.

type Role struct {
	bun.BaseModel `bun:"table:roles,alias:r"`

	ID   int16  `bun:"id,pk,autoincrement" json:"id"`
	Name string `bun:"name,notnull" json:"name"`
}

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID              string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	CompanyID       string     `bun:"company_id,notnull"`
	Email           string     `bun:"email,notnull"`
	PasswordHash    string     `bun:"password_hash,notnull"`
	RoleID          int16      `bun:"role_id,notnull"`
	IsActive        bool       `bun:"is_active,notnull"`
	TOTPEnabled     bool       `bun:"totp_enabled,notnull"`
	TOTPSecretEnc   *string    `bun:"totp_secret_enc"`
	TOTPConfirmedAt *time.Time `bun:"totp_confirmed_at"`
	CreatedAt       time.Time  `bun:"created_at,nullzero,default:now()"`
	UpdatedAt       time.Time  `bun:"updated_at,nullzero,default:now()"`
}

type RecoveryCode struct {
	bun.BaseModel `bun:"table:user_recovery_codes,alias:urc"`

	ID        string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	UserID    string     `bun:"user_id,notnull"`
	CodeHash  string     `bun:"code_hash,notnull"`
	UsedAt    *time.Time `bun:"used_at"`
	CreatedAt time.Time  `bun:"created_at,nullzero,default:now()"`
}

type RefreshToken struct {
	bun.BaseModel `bun:"table:refresh_tokens,alias:rt"`

	ID           string     `bun:"id,pk,type:uuid,default:gen_random_uuid(),nullzero"`
	UserID       string     `bun:"user_id,notnull"`
	TokenHash    string     `bun:"token_hash,notnull"`
	ReplacedByID *string    `bun:"replaced_by_id"`
	ExpiresAt    time.Time  `bun:"expires_at,notnull"`
	RevokedAt    *time.Time `bun:"revoked_at"`
	UserAgent    *string    `bun:"user_agent"`
	IP           *string    `bun:"ip,type:inet"`
	CreatedAt    time.Time  `bun:"created_at,nullzero,default:now()"`
}
