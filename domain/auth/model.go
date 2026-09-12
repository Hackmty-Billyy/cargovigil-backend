package auth

import "time"

type Role struct {
	ID   int16
	Name string
}

type User struct {
	ID              string
	Email           string
	PasswordHash    string
	RoleID          int16
	IsActive        bool
	TOTPEnabled     bool
	TOTPSecretEnc   *string
	TOTPConfirmedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RecoveryCode struct {
	ID        string
	UserID    string
	CodeHash  string
	UsedAt    *time.Time
	CreatedAt time.Time
}

type RefreshToken struct {
	ID           string
	UserID       string
	TokenHash    string
	ReplacedByID *string
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	UserAgent    *string
	IP           *string
	CreatedAt    time.Time
}
