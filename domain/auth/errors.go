package auth

import "errors"

var (
	ErrNotFound = errors.New("auth: record not found")

	ErrInvalidCredentials  = errors.New("auth: invalid credentials")
	ErrUserInactive        = errors.New("auth: user inactive")
	ErrInvalidToken        = errors.New("auth: invalid or expired token")
	ErrInvalidTOTPCode     = errors.New("auth: invalid totp or recovery code")
	ErrTOTPAlreadyEnabled  = errors.New("auth: totp already enabled")
	ErrTOTPNotEnrolled     = errors.New("auth: totp not enrolled")
	ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")
	ErrRefreshTokenExpired = errors.New("auth: refresh token expired")
	ErrSessionCompromised  = errors.New("auth: refresh token reuse detected, all sessions revoked")
)
