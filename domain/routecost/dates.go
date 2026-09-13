package routecost

import "time"

const (
	dateLayout     = "2006-01-02"
	dateTimeLayout = time.RFC3339
)

// parseTimestamp accepts either a full RFC3339 timestamp or a bare date, so
// the API is usable both from the dashboard (which sends timestamps) and from
// quick manual captures (which send "2026-09-12").
func parseTimestamp(raw string) (time.Time, error) {
	if t, err := time.Parse(dateTimeLayout, raw); err == nil {
		return t, nil
	}
	return time.Parse(dateLayout, raw)
}

func parseOptionalTimestamp(raw *string) (*time.Time, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	t, err := parseTimestamp(*raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
