package mcp

import "github.com/kang-sw/devenv/internal/wsagent"

// agentBackends binds config admission to the daemon's one runner registry.
// A standalone server has no process registry and retains builtin-only reads.
func (s *Server) agentBackends() []map[string]wsagent.BackendRegistration {
	if source, ok := s.pi.agents.(interface {
		BackendRegistrations() map[string]wsagent.BackendRegistration
	}); ok {
		return []map[string]wsagent.BackendRegistration{source.BackendRegistrations()}
	}
	return nil
}
