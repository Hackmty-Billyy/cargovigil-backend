package treasury

import "time"

const dateLayout = "2006-01-02"

func parseDate(raw string) (time.Time, error) {
	return time.Parse(dateLayout, raw)
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
