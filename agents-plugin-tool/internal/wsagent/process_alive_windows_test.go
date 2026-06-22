//go:build windows

package wsagent

import (
	"testing"

	"golang.org/x/sys/windows"
)

// TestOpenErrorMeansAliveAccessDenied verifies that ERROR_ACCESS_DENIED is
// classified as "alive" (mirrors Unix EPERM→alive).
func TestOpenErrorMeansAliveAccessDenied(t *testing.T) {
	if !openErrorMeansAlive(windows.ERROR_ACCESS_DENIED) {
		t.Error("openErrorMeansAlive(ERROR_ACCESS_DENIED) = false, want true")
	}
}

// TestOpenErrorMeansAliveInvalidParameter verifies that ERROR_INVALID_PARAMETER
// (no such PID) is classified as "not alive".
func TestOpenErrorMeansAliveInvalidParameter(t *testing.T) {
	if openErrorMeansAlive(windows.ERROR_INVALID_PARAMETER) {
		t.Error("openErrorMeansAlive(ERROR_INVALID_PARAMETER) = true, want false")
	}
}

// TestOpenErrorMeansAliveNil verifies that a nil error is not "alive" via this
// helper (the nil case means OpenProcess succeeded and we go to WaitForSingle).
func TestOpenErrorMeansAliveNil(t *testing.T) {
	if openErrorMeansAlive(nil) {
		t.Error("openErrorMeansAlive(nil) = true, want false")
	}
}
