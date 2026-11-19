package mcp

import "github.com/kang-sw/devenv/internal/wsagent"

// AgentBackendRegistry exposes the daemon runner's registered backend
// descriptors without granting the lead runner handle (PiAgents). Worker
// connections receive this so they can resolve their backend descriptor and
// profiles while keeping pi.*/agent.* calls on the token-forwarding path.
type AgentBackendRegistry interface {
	BackendRegistrations() map[string]wsagent.BackendRegistration
}

// agentBackends binds config admission and connection descriptors to the
// daemon's one runner registry. A standalone server has no process registry
// and retains builtin-only reads.
func (s *Server) agentBackends() []map[string]wsagent.BackendRegistration {
	if s.agentBackendSource != nil {
		return []map[string]wsagent.BackendRegistration{s.agentBackendSource.BackendRegistrations()}
	}
	if source, ok := s.pi.agents.(AgentBackendRegistry); ok {
		return []map[string]wsagent.BackendRegistration{source.BackendRegistrations()}
	}
	return nil
}
