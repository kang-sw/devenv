package wsrationale

import (
	"fmt"
	"strings"
	"time"
)

// parseDateBounds validates the since/until arguments as YYYY-MM-DD and returns
// them normalized (empty when unset). A malformed date is an error, not an
// ignored filter.
func parseDateBounds(since, until string) (string, string, error) {
	s, err := normalizeDate("since", since)
	if err != nil {
		return "", "", err
	}
	u, err := normalizeDate("until", until)
	if err != nil {
		return "", "", err
	}
	return s, u, nil
}

func normalizeDate(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", fmt.Errorf("%s must be a YYYY-MM-DD date: %q", name, value)
	}
	return value, nil
}

// dateInBounds compares ISO dates lexicographically, which is a correct date
// order for the YYYY-MM-DD form. An empty record date passes both bounds so a
// record with an unknown date is never silently dropped.
func dateInBounds(date, since, until string) bool {
	if date == "" {
		return true
	}
	if since != "" && date < since {
		return false
	}
	if until != "" && date > until {
		return false
	}
	return true
}
