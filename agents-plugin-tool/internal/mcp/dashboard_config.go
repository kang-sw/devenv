package mcp

import (
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// The dashboard keys live in a "dashboard."-namespace manifest each package
// ships beside its own manifest. ws-mcp declares no built-in value for them:
// an instance whose launcher did not load that manifest (or whose manifest was
// rejected) has the dashboard off.
const (
	DashboardEnabledKey = "dashboard.enabled"
	DashboardPortKey    = "dashboard.port"
)

// DashboardSettings is the dashboard configuration a stdio server reads once
// at startup.
type DashboardSettings struct {
	// Enabled is true only when dashboard.enabled is declared by a loaded
	// manifest and resolves to true.
	Enabled bool
	// Port is dashboard.port, or wsdashboard.DefaultPort when undeclared.
	Port int
}

// DashboardSettings resolves the dashboard keys without a session: startup
// has none, so only the project, global, and built-in values apply. A stored
// value that fails its declared type resolves to the manifest default, as
// config.get does.
func (s *Server) DashboardSettings() DashboardSettings {
	out := DashboardSettings{Port: wsdashboard.DefaultPort}
	if _, ok := s.declared.lookup(DashboardEnabledKey); !ok {
		return out
	}
	if got, err := s.resolveConfigGet("", DashboardEnabledKey); err == nil {
		out.Enabled, _ = got.Value.(bool)
	}
	if _, ok := s.declared.lookup(DashboardPortKey); ok {
		if got, err := s.resolveConfigGet("", DashboardPortKey); err == nil {
			if port, ok := got.Value.(int64); ok {
				out.Port = int(port)
			}
		}
	}
	return out
}
