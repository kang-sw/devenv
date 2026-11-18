package wsagent

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wspackage"
)

// hostedPluginPackages are the installed workflow plugins a hosted Claude lead
// turns off. Hosted processes themselves use the daemon's build-owned packages.
var hostedPluginPackages = []string{"ws", "wsflow"}

// ResolveHostedPackage reads only the daemon's global package setting at process
// start. Installed roots, caller resource paths and legacy registries are ignored.
func ResolveHostedPackage() (name, warning string, err error) {
	resolver := wsconfig.NewResolver(wsconfig.Options{}, map[string]string{wsconfig.HostedPluginPackageKey: "wsflow"}, nil, nil)
	selected, configErr := resolver.Get("", wsconfig.HostedPluginPackageKey)
	if configErr != nil {
		return "wsflow", fmt.Sprintf("cannot read global %s: %v; using wsflow.", wsconfig.HostedPluginPackageKey, configErr), nil
	}
	name = selected.Value
	if name != "ws" && name != "wsflow" {
		return "wsflow", fmt.Sprintf("invalid %s %q; using wsflow.", wsconfig.HostedPluginPackageKey, name), nil
	}
	return name, "", nil
}

// HostedPluginEnvironment reads package policy from build-owned plugin.json.
// root is a staged package/workflow root used only for path substitution, never
// a source of manifest bytes. Blank values clear undeclared starter policy.
func HostedPluginEnvironment(name, root string) (map[string]string, error) {
	source, err := wspackage.Open(name)
	if err != nil {
		return nil, err
	}
	raw, err := fs.ReadFile(source, ".claude-plugin/plugin.json")
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Name    string `json:"name"`
		Servers map[string]struct {
			Env map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	if manifest.Name != name {
		return nil, fmt.Errorf("workflow package %s manifest declares %q", name, manifest.Name)
	}
	env := map[string]string{
		"WS_RSRC_ROOT":     filepath.Join(root, "rsrc"),
		"WS_SKILLS_ROOT":   filepath.Join(root, "skills"),
		"WS_MCP_NAMESPACE": "", "WS_MCP_SETUP_TOOL": "", "WS_MCP_NO_AGENT": "",
		"WS_MCP_CLI_NAME": "", "WS_MCP_CONFIG_MANIFESTS": "",
	}
	for key, value := range manifest.Servers[name].Env {
		if strings.Contains(value, "${CLAUDE_PLUGIN_ROOT}") {
			value = strings.ReplaceAll(filepath.FromSlash(value), "${CLAUDE_PLUGIN_ROOT}", root)
		}
		env[key] = value
	}
	return env, nil
}
