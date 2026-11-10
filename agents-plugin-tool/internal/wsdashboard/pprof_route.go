package wsdashboard

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"strings"
	"time"
)

// PprofEnv names the environment variable that serves net/http/pprof under
// /debug/pprof/ on the daemon, so its CPU use can be profiled at function
// level and its waits attributed: with it on, the daemon also records the
// mutex and block profiles (enableContentionProfiles), which are otherwise
// empty. It is read by `ws-mcp dashboard` only; 1/true/yes/on turns it on, and
// anything else (or unset) serves no such route.
const PprofEnv = "WS_MCP_DASHBOARD_PPROF"

// The contention sampling the pprof opt-in turns on. The block profile
// records every wait of a goroutine of at least pprofBlockRate (channel,
// select, and lock waits; shorter ones in proportion), so a stall of a
// tenth of a second or more is never missed while the many sub-millisecond
// waits of normal serving cost little. The mutex profile samples one in
// pprofMutexFraction contended unlocks, attributing a wait to the holder's
// release; a stall makes every queued request one such event, so a rare
// one still lands in it.
const (
	pprofBlockRate     = int(time.Millisecond) // in nanoseconds
	pprofMutexFraction = 5
)

// enableContentionProfiles turns on the mutex and block profiles for the
// whole process; /debug/pprof/mutex and /debug/pprof/block serve them.
func enableContentionProfiles() {
	runtime.SetMutexProfileFraction(pprofMutexFraction)
	runtime.SetBlockProfileRate(pprofBlockRate)
}

// PprofEnabled reports whether a PprofEnv value turns the pprof routes on.
func PprofEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// pprofPrefix is the path the pprof routes are served under.
const pprofPrefix = "/debug/pprof/"

// pprofHandler serves the pprof routes to a browser or tool on this machine
// only: in LAN mode the access guard admits a LAN token, which must not open
// profiles (a goroutine dump or the command line shows more than any page),
// so this refuses every request that is not a local viewer whatever it
// carries.
//
// The mutex and block profiles are served by pprof.Index; the daemon turns
// their sampling on when it starts with the routes (enableContentionProfiles).
//
// It also serves the daemon's live-update counters (streamStats) as JSON at
// pprofPrefix+"streams".
func pprofHandler(streams http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(pprofPrefix+"streams", streams)
	mux.HandleFunc(pprofPrefix, pprof.Index)
	mux.HandleFunc(pprofPrefix+"cmdline", pprof.Cmdline)
	mux.HandleFunc(pprofPrefix+"profile", pprof.Profile)
	mux.HandleFunc(pprofPrefix+"symbol", pprof.Symbol)
	mux.HandleFunc(pprofPrefix+"trace", pprof.Trace)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localViewer(r) {
			http.Error(w, "pprof is served to this machine only", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
