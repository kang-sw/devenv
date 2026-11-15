//go:build !darwin

package wsdashboard

import "errors"

// launchdSpawn exists only on macOS; elsewhere SpawnDetached always uses
// startDetached.
func launchdSpawn(string, []string, string) error { return errors.New("launchd is macOS only") }
