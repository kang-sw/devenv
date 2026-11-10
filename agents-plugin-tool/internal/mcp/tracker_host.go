package mcp

import (
	"context"
	"sync"

	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// trackerLink is this process's handle on the machine's tracker stores. The
// host is created on first use and kept for the process lifetime, so a
// store this process wrote to stays open and wsledger's idle push fires.
type trackerLink struct {
	once sync.Once
	host *wstrackerhost.Host
	err  error
	// opts configures the host; tests set it before first use. The zero
	// value is the real machine root (beside the global config, so it
	// follows $WS_CONFIG_HOME).
	opts wstrackerhost.Options
	// checks tracks the background session-start checks, so a test can
	// wait for them.
	checks sync.WaitGroup
}

// trackerHost returns this process's tracker host.
func (s *Server) trackerHost() (*wstrackerhost.Host, error) {
	s.tracker.once.Do(func() {
		// The host follows the caller: its root beside the caller's global
		// config, its git under the caller's environment.
		opts := s.tracker.opts
		if opts.Root == "" {
			opts.Root, s.tracker.err = wstrackerhost.Root(s.configOptions())
			if s.tracker.err != nil {
				return
			}
		}
		if opts.Environ == nil {
			opts.Environ = s.environ
		}
		s.tracker.host, s.tracker.err = wstrackerhost.New(opts)
	})
	return s.tracker.host, s.tracker.err
}

// startTrackerSessionCheck runs the lead-session-start check (a throttled
// fetch of every registered store) in the background when the registry
// holds a store, whatever workflow.tracker_enabled says: the knob gates only
// manual text. It never delays or fails the manual; a slow remote, a
// credential the non-interactive runner cannot supply, or any error leaves
// the stores as they were.
func (s *Server) startTrackerSessionCheck() {
	h, err := s.trackerHost()
	if err != nil || !h.HasStores() {
		return
	}
	s.tracker.checks.Add(1)
	go func() {
		defer s.tracker.checks.Done()
		defer func() { _ = recover() }()
		h.CheckAll(context.Background())
	}()
}
