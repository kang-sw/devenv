//go:build !windows

package main

// startParentDeathWatch is a no-op on POSIX: os.execvpe replaces the launcher
// with the ws-mcp binary, so the Go server is already the process the host
// supervises — there is no intermediate parent to outlive. Kept as a build-
// tagged stub so cmd/ws-mcp compiles identically on every platform.
func startParentDeathWatch() {}
