package wsdashboard

import (
	"net/http"
	"sync/atomic"
)

// streamStats counts the live-update work since the daemon started: the
// wakes by source, the stream signals they delivered, and the renders, so a
// measurement can tell wake fan-out from render cost. The counters are
// always kept (an atomic add each); only the pprof opt-in serves them, at
// pprofPrefix+"streams" (pprofHandler).
type streamStats struct {
	// Wakes by source: the index's change hook, the agent runner, the git
	// watcher, an inbox write, and the tracker.
	WakeSession, WakeAgent, WakeGit, WakeInbox, WakeTracker atomic.Int64
	// Signals counts the wake signals delivered to streams.
	Signals atomic.Int64
	// SessionRenders and IndexRenders count page renders actually run (a
	// read of the page's state and a render of every region).
	SessionRenders, IndexRenders atomic.Int64
	// Shared counts stream rounds served from another stream's render.
	Shared atomic.Int64
	// RefreshSkips counts refresh ticks that found nothing to re-render.
	RefreshSkips atomic.Int64
	// Sends counts region events written to streams.
	Sends atomic.Int64
}

// snapshot is the counters as a JSON-ready map.
func (s *streamStats) snapshot() map[string]int64 {
	return map[string]int64{
		"wake_session":    s.WakeSession.Load(),
		"wake_agent":      s.WakeAgent.Load(),
		"wake_git":        s.WakeGit.Load(),
		"wake_inbox":      s.WakeInbox.Load(),
		"wake_tracker":    s.WakeTracker.Load(),
		"signals":         s.Signals.Load(),
		"session_renders": s.SessionRenders.Load(),
		"index_renders":   s.IndexRenders.Load(),
		"shared":          s.Shared.Load(),
		"refresh_skips":   s.RefreshSkips.Load(),
		"sends":           s.Sends.Load(),
	}
}

func (s *streamStats) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.snapshot())
}
