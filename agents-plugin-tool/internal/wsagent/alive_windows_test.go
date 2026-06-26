//go:build windows

package wsagent

// aliveForTest reports whether pid refers to a live process, reusing the
// package's own Windows liveness probe. processAlive now disambiguates an
// exited-but-unreaped process via a zero-timeout wait, so this reports false
// promptly once cancelAsyncProcessTree reaps the subtree.
func aliveForTest(pid int) bool {
	alive, err := processAlive(pid)
	return err == nil && alive
}
