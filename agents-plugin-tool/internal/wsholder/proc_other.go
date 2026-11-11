//go:build !linux && !darwin && !windows

package wsholder

import (
	"errors"
	"time"
)

// processStartTime is unknown on this OS, so a guarded kill is refused.
func processStartTime(int) (time.Time, error) {
	return time.Time{}, errors.New("process start times are not readable on this OS")
}
