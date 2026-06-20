//go:build windows

package wsagent

// aliveForTest reports whether pid refers to a live process, reusing the
// package's own Windows liveness probe. The zombie/exited-handle nuance is the
// deferred Phase 3 concern noted in mental-model/named-agent-runtime.md
// (Technical Debt); acceptable for the Phase 3 host run of this subtree test.
func aliveForTest(pid int) bool {
	alive, err := processAlive(pid)
	return err == nil && alive
}
