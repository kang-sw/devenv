package mcp

import (
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// The dashboard keys live in a "dashboard."-namespace manifest each package
// ships beside its own manifest. ws-mcp declares no built-in value for them:
// an instance whose launcher did not load that manifest (or whose manifest was
// rejected) has the dashboard off.
const (
	DashboardEnabledKey = "dashboard.enabled"
	DashboardPortKey    = "dashboard.port"
	// DashboardLANKey turns the daemon's LAN mode on. It is global-only: one
	// daemon serves every session on the machine, so one project must not be
	// able to turn LAN on for all of them, and the value must not depend on
	// which session started the daemon. The resolver therefore reads only the
	// global layer and the manifest default for it, and refuses a write at any
	// other scope.
	DashboardLANKey = "dashboard.lan_enabled"
)

// manifestGlobalOnlyKeys are manifest-declared keys that pin themselves
// global-only through wsconfig. The manifest namespace check treats a
// registered key as ws-mcp's own, so it skips these.
var manifestGlobalOnlyKeys = map[string]struct{}{DashboardLANKey: {}}

func init() { wsconfig.RegisterGlobalOnly(DashboardLANKey) }

// DashboardSettings is the dashboard configuration a stdio server reads once
// at startup.
type DashboardSettings struct {
	// Enabled is true only when dashboard.enabled is declared by a loaded
	// manifest and resolves to true.
	Enabled bool
	// Port is dashboard.port, or wsdashboard.DefaultPort when undeclared.
	Port int
	// LAN is the global dashboard.lan_enabled value, false when the key is
	// undeclared.
	LAN bool
}

// DashboardSettings resolves the dashboard keys without a session: startup
// has none, so only the project, global, and built-in values apply (the LAN
// key reads the global and built-in values only). A stored
// value that fails its declared type resolves to the manifest default, as
// config.get does.
func (s *Server) DashboardSettings() DashboardSettings {
	out := DashboardSettings{Port: wsdashboard.DefaultPort}
	if !s.dashboardDeclared() {
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
	if _, ok := s.declared.lookup(DashboardLANKey); ok {
		if got, err := s.resolveConfigGet("", DashboardLANKey); err == nil {
			out.LAN, _ = got.Value.(bool)
		}
	}
	return out
}
