package wsdashboard

import (
	"net/http"
	"net/http/pprof"
	"strings"
)

// PprofEnv names the environment variable that serves net/http/pprof under
// /debug/pprof/ on the daemon, so its CPU use can be profiled at function
// level. It is read by `ws-mcp dashboard` only; 1/true/yes/on turns it on, and
// anything else (or unset) serves no such route.
const PprofEnv = "WS_MCP_DASHBOARD_PPROF"

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
func pprofHandler() http.Handler {
	mux := http.NewServeMux()
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
