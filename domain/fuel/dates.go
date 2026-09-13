package fuel

import "time"

const dateLayout = time.RFC3339

// parseTimestamp accepts RFC3339 (unlike treasury's dateLayout, fuel_indexes
// and trip_fuel_logs use TIMESTAMPTZ columns, not DATE).
func parseTimestamp(raw string) (time.Time, error) {
	return time.Parse(dateLayout, raw)
}
