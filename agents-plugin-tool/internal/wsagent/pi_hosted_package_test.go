package wsagent

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiLeadSourceSiblingUsesSelectedPackage(t *testing.T) {
	for _, owner := range []string{"ws", "wsflow"} {
		t.Run(owner, func(t *testing.T) {
			spec, sibling := piLeadFixture(t)
			piLeadBundle(t, sibling)
			root := filepath.Join(spec.Root, owner)
			noAgent := ""
			if owner == "wsflow" {
				noAgent = "1"
			}
			piLeadFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"), fmt.Sprintf(`{"name":%q,"mcpServers":{%q:{"env":{"WS_MCP_NAMESPACE":%q,"WS_MCP_NO_AGENT":%q,"WS_MCP_CONFIG_MANIFESTS":"${CLAUDE_PLUGIN_ROOT}/config-manifest.json;${CLAUDE_PLUGIN_ROOT}/dashboard-config-manifest.json"}}}}`, owner, owner, owner, noAgent))
			piBundledPluginRoot = func() (string, string, error) { return root, "Hosted plugin fallback notice", nil }
			spec.Lead.PluginIsolation = "isolated"
			spec.Env = []string{"WS_MCP_NAMESPACE=starter", "WS_MCP_NO_AGENT=starter", "WS_SKILLS_ROOT=starter", "WS_PI_WORKFLOW_ROOT=starter"}
			var warning string
			spec.Lead.PluginWarning = func(text string) { warning = text }
			if _, _, err := piLeadLaunch("pi", nil, spec); err != nil {
				t.Fatal(err)
			}
			env := map[string]string{}
			for _, entry := range piLeadEnv(spec) {
				name, value, _ := strings.Cut(entry, "=")
				env[name] = value
			}
			for name, want := range map[string]string{
				"WS_PI_WORKFLOW_ROOT":     root,
				"WS_MCP_NAMESPACE":        owner,
				"WS_MCP_NO_AGENT":         noAgent,
				"WS_RSRC_ROOT":            filepath.Join(root, "rsrc"),
				"WS_SKILLS_ROOT":          filepath.Join(root, "skills"),
				"WS_MCP_CONFIG_MANIFESTS": filepath.Join(root, "config-manifest.json") + ";" + filepath.Join(root, "dashboard-config-manifest.json"),
			} {
				if env[name] != want {
					t.Fatalf("%s = %q, want %q", name, env[name], want)
				}
			}
			if warning != "Hosted plugin fallback notice" {
				t.Fatalf("package warning lost: %q", warning)
			}
		})
	}
}

func TestPiMCPOnlyIgnoresPackageAndIsolationWarnings(t *testing.T) {
	for _, warning := range []string{"", "Hosted plugin wsflow: selected package root is unavailable; using daemon package ws instead.", piNoSDKWarning} {
		if PiMCPOnly(Record{Backend: BackendPi, Lead: true, PluginWarning: warning}, nil) {
			t.Fatalf("native extension incorrectly classified as MCP-only: %q", warning)
		}
	}
	if !PiMCPOnly(Record{Backend: BackendPi, Lead: true, PluginWarning: "Hosted package fallback. Bundled Pi extension unavailable; using ws MCP bridge"}, nil) {
		t.Fatal("extension bridge fallback must stay MCP-only")
	}
}
