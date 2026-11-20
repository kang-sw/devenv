package mcp

import "strings"

// planUsageActivityHost is the optional in-memory daemon capability used by a
// user-started lead's tool calls. The daemon implementation only schedules the
// existing background refresh and must return without waiting for its store.
type planUsageActivityHost interface {
	PlanUsageActivity()
}

// signalPlanUsageActivity reports a user-started lead call when the detected
// harness's backend descriptor opts in. Hosted sessions return from
// dashboardTouch before reaching this method and use their settled-turn feed.
func (s *Server) signalPlanUsageActivity() {
	s.planUsageActivityOnce.Do(func() {
		harness := s.currentHarness()
		if harness == "" {
			return
		}
		for _, registrations := range s.agentBackends() {
			for _, registration := range registrations {
				if registration.PlanUsageActivity && strings.EqualFold(strings.TrimSpace(registration.ManualHarness), harness) {
					s.planUsageActivity = true
					return
				}
			}
		}
	})
	if !s.planUsageActivity {
		return
	}
	if host, ok := s.live.host.(planUsageActivityHost); ok {
		host.PlanUsageActivity()
	}
}
