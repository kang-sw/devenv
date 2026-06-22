//go:build windows

package wsstate

import (
	"testing"

	"golang.org/x/sys/windows"
)

// TestOpenErrorMeansAliveAccessDenied verifies ERROR_ACCESS_DENIED → alive.
func TestOpenErrorMeansAliveAccessDenied(t *testing.T) {
	if !openErrorMeansAlive(windows.ERROR_ACCESS_DENIED) {
		t.Error("openErrorMeansAlive(ERROR_ACCESS_DENIED) = false, want true")
	}
}

// TestOpenErrorMeansAliveInvalidParameter verifies ERROR_INVALID_PARAMETER → not alive.
func TestOpenErrorMeansAliveInvalidParameter(t *testing.T) {
	if openErrorMeansAlive(windows.ERROR_INVALID_PARAMETER) {
		t.Error("openErrorMeansAlive(ERROR_INVALID_PARAMETER) = true, want false")
	}
}
