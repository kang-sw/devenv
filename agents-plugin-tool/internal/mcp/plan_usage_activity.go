package mcp

import (
	"strings"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// planUsageActivityHost is the optional in-memory daemon capability used by a
// user-started agy lead's tool calls. The daemon implementation only schedules
// the existing agy refresh and must return without waiting for its store.
type planUsageActivityHost interface {
	PlanUsageActivity()
}

var _ planUsageActivityHost = (*wsdashboard.Daemon)(nil)

// planUsageActivityEnabled reports whether the detected harness's backend
// descriptor opts user-started lead calls into the agy plan-usage activity feed.
func (s *Server) planUsageActivityEnabled() bool {
	harness := s.currentHarness()
	if harness == "" {
		return false
	}
	s.planUsageActivityOnce.Do(func() {
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

// signalPlanUsageActivity hands an admitted user-started agy lead call to the
// optional daemon host. dashboardTouch excludes hosted sessions first; they use
// their settled-turn agy feed instead.
func (s *Server) signalPlanUsageActivity() {
	if host, ok := s.live.host.(planUsageActivityHost); ok {
		host.PlanUsageActivity()
	}
}
