package logistics

import "errors"

var (
	ErrNotFound        = errors.New("logistics: trip not found")
	ErrInvalidVehicle  = errors.New("logistics: vehicle_type must be truck, ship or plane")
	ErrInvalidDates    = errors.New("logistics: invalid departure or arrival date")
	ErrNoFuelIndex     = errors.New("logistics: no fuel index recorded for this vehicle type yet")
	ErrInvalidCurrency = errors.New("logistics: unsupported currency")
)
