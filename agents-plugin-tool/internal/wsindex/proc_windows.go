//go:build windows

package wsindex

import "os/exec"

// configureKill keeps the default kill on Windows; WaitDelay still bounds the
// wait for a lingering child.
func configureKill(cmd *exec.Cmd) {}
