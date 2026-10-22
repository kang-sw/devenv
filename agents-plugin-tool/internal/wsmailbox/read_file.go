package wsmailbox

import "time"

// readRetryBudget bounds how long readFileRetrying retries a transient open
// failure; it matches replaceFile's write-side budget so a reader outlasts a
// replace in flight.
const readRetryBudget = 2 * time.Second

// readRetryMaxBackoff caps the doubling back-off. replaceFile's loop doubles
// past its 200ms check once (160ms -> 320ms); this one clamps.
const readRetryMaxBackoff = 200 * time.Millisecond

// readFileRetrying calls read(path) until it succeeds, fails with an error
// transient does not classify as transient, or the budget elapses, in which
// case the last error is returned. Back-off starts at 5ms and doubles up to
// readRetryMaxBackoff. Errors are returned unwrapped so callers'
// os.IsNotExist checks keep working.
func readFileRetrying(
	path string,
	read func(string) ([]byte, error),
	transient func(error) bool,
	now func() time.Time,
	sleep func(time.Duration),
) ([]byte, error) {
	deadline := now().Add(readRetryBudget)
	backoff := 5 * time.Millisecond
	for {
		raw, err := read(path)
		if err == nil || !transient(err) || now().After(deadline) {
			return raw, err
		}
		sleep(backoff)
		backoff = min(backoff*2, readRetryMaxBackoff)
	}
}
