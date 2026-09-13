package routecost

import "errors"

var (
	ErrNotFound            = errors.New("routecost: record not found")
	ErrInvalidReference    = errors.New("routecost: vehicle, route, client or contract does not belong to this company")
	ErrInvalidStatus       = errors.New("routecost: invalid trip status")
	ErrInvalidTransition   = errors.New("routecost: trip is already closed")
	ErrInvalidEventType    = errors.New("routecost: invalid friction event type")
	ErrInvalidDates        = errors.New("routecost: estimated arrival must be after departure")
	ErrFrictionClosed      = errors.New("routecost: friction is already closed")
	ErrFundReleased        = errors.New("routecost: contingency fund was already released")
	ErrUnsupportedCurrency = errors.New("routecost: unsupported currency pair for conversion")
	ErrInvalidRate         = errors.New("routecost: idle_hourly_rate must be zero or positive")
	ErrDuplicateTracking   = errors.New("routecost: tracking_code already exists for this company")
)
