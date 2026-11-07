package wsdashboard

import "os"

// BuildTime is a ws-mcp build's identity for ordering two builds of the same
// version: the modification time of the executable the build produced, in
// Unix milliseconds. Zero means unknown, and an unknown build time never
// replaces a daemon.
//
// The time is read from the original executable, never from the copy
// DaemonExecutable may stage on Windows: a copy's mtime is the copy time, and
// a reused copy would keep reporting the first copy's time. The serving
// ws-mcp reads it and passes it to the daemon it starts (the --build-time
// flag), so the daemon reports what its client measured.
//
// Ordering by build time rather than "a different build replaces" keeps the
// order total: the Claude plugin runtime and the Pi runtime run different
// ws-mcp binaries of one version side by side, and each must leave the
// other's newer daemon alone.
func ExecutableBuildTime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if ms := info.ModTime().UnixMilli(); ms > 0 {
		return ms
	}
	return 0
}
