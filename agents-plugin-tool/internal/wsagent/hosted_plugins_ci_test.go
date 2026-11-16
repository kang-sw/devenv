package wsagent

import (
	"path/filepath"
	"testing"
)

func TestHostedPluginEnvironmentUsesNativeTemplatePaths(t *testing.T) {
	root := t.TempDir()
	writeLeadPluginFixture(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{
		"name":"wsflow",
		"mcpServers":{"wsflow":{"env":{
			"WS_MCP_CONFIG_MANIFESTS":"${CLAUDE_PLUGIN_ROOT}/config-manifest.json;${CLAUDE_PLUGIN_ROOT}/nested/dashboard-config-manifest.json",
			"WS_MCP_CUSTOM":"${CLAUDE_PLUGIN_ROOT}/nested/custom",
			"WS_MCP_ENDPOINT":"https://example.invalid/a/b",
			"WS_MCP_NAMESPACE":"wsflow"
		}}}
	}`)
	env, err := HostedPluginEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"WS_MCP_CONFIG_MANIFESTS": filepath.Join(root, "config-manifest.json") + ";" + filepath.Join(root, "nested", "dashboard-config-manifest.json"),
		"WS_MCP_CUSTOM":           filepath.Join(root, "nested", "custom"),
		"WS_RSRC_ROOT":            filepath.Join(root, "rsrc"),
		"WS_SKILLS_ROOT":          filepath.Join(root, "skills"),
		"WS_MCP_ENDPOINT":         "https://example.invalid/a/b",
		"WS_MCP_NAMESPACE":        "wsflow",
	} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}
}
