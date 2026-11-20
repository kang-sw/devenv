package mcp

import "strings"

// planUsageActivityHost is the optional in-memory daemon capability used by a
// user-started lead's tool calls. The daemon implementation only schedules the
// existing background refresh and must return without waiting for its store.
type planUsageActivityHost interface {
	PlanUsageActivity()
}

// planUsageActivityEnabled reports whether the detected harness's backend
// descriptor opts user-started lead calls into the plan-usage activity feed.
func (s *Server) planUsageActivityEnabled() bool {
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
	return s.planUsageActivity
}

// signalPlanUsageActivity hands an admitted user-started lead call to the
// optional daemon host. dashboardTouch excludes hosted sessions first; they
// use their settled-turn feed instead.
func (s *Server) signalPlanUsageActivity() {
	if host, ok := s.live.host.(planUsageActivityHost); ok {
		host.PlanUsageActivity()
	}
}
