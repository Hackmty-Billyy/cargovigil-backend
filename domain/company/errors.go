package company

import "errors"

var (
	ErrNotFound    = errors.New("company: record not found")
	ErrInvalidRole = errors.New("company: invalid role for teammate invite")
)
