package company

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

func parseContractDates(startsOn, endsOn *string) (*time.Time, *time.Time, error) {
	start, err := parseOptionalDate(startsOn)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid starts_on: %w", err)
	}
	end, err := parseOptionalDate(endsOn)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid ends_on: %w", err)
	}
	return start, end, nil
}

func parseOptionalDate(raw *string) (*time.Time, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, *raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
