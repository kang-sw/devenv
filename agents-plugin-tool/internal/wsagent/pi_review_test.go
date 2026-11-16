package wsagent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPiLeadManifestEnvironmentBoundary(t *testing.T) {
	spec, sibling := piLeadFixture(t)
	piLeadBundle(t, sibling)
	root := filepath.Join(spec.Root, "agents-plugin-wsflow")
	piLeadFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{"name":"wsflow","mcpServers":{"wsflow":{"env":{"WS_MCP_NAMESPACE":"wsflow","WS_MCP_CUSTOM":"${CLAUDE_PLUGIN_ROOT}/custom","WS_RSRC_ROOT":"/manifest/rsrc","WS_SKILLS_ROOT":"/manifest/skills","PATH":"/manifest/bin","NODE_OPTIONS":"--manifest-option","WS_HOSTED_SESSION_KEY":"manifest-key","WS_PI_WORKFLOW_ROOT":"/manifest/workflow"}}}}`)
	spec.Lead.PluginIsolation = "isolated"
	spec.Lead.Env = map[string]string{EnvHostedSessionKey: "daemon-key"}
	spec.Env = []string{"PATH=/daemon/bin", "NODE_OPTIONS=--daemon-option"}
	if _, _, err := piLeadLaunch("pi", nil, spec); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, entry := range piLeadEnv(spec) {
		name, value, _ := strings.Cut(entry, "=")
		env[name] = value
	}
	for name, want := range map[string]string{
		"WS_MCP_NAMESPACE":    "wsflow",
		"WS_MCP_CUSTOM":       filepath.Join(root, "custom"),
		"WS_RSRC_ROOT":        filepath.Join(root, "rsrc"),
		"WS_SKILLS_ROOT":      filepath.Join(root, "skills"),
		"WS_PI_WORKFLOW_ROOT": root,
		EnvHostedSessionKey:   "daemon-key",
		"PATH":                "/daemon/bin",
		"NODE_OPTIONS":        "--daemon-option",
	} {
		if env[name] != want {
			t.Errorf("%s = %q, want %q", name, env[name], want)
		}
	}
}

func TestPiLeadFallbackWarningClassifiesMCPOnly(t *testing.T) {
	spec, _ := piLeadFixture(t)
	spec.Lead.PluginIsolation = "isolated"
	spec.Lead.Env = map[string]string{EnvHostedSessionKey: "daemon-key"}
	var warning string
	spec.Lead.PluginWarning = func(text string) { warning = text }
	spec.Lead.WSMCP = func() (MCPCommand, error) {
		return MCPCommand{Command: "ws-mcp"}, nil
	}
	if _, _, err := piLeadLaunch("pi", nil, spec); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning, piExtensionFallbackMarker) {
		t.Fatalf("fallback warning lacks classification marker: %q", warning)
	}
	if !PiMCPOnly(Record{Backend: BackendPi, Lead: true, PluginWarning: warning + " " + piNoSDKWarning}, nil) {
		t.Fatalf("actual bridge fallback not classified as MCP-only: %q", warning)
	}
}
