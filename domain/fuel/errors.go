package fuel

import "errors"

var (
	ErrNotFound         = errors.New("fuel: record not found")
	ErrDuplicateRule    = errors.New("fuel: a surcharge rule already exists for this fuel_type + region")
	ErrInvalidFuelType  = errors.New("fuel: unrecognized fuel_type")
	ErrInsufficientData = errors.New("fuel: not enough historical trip_fuel_logs for this route + fuel_type to simulate")
	ErrInvalidVariation = errors.New("fuel: price_variation_percentage must be a finite number")
)
