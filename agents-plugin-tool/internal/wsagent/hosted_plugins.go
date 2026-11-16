package wsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

const hostedPluginRootsFile = "hosted-plugin-roots.json"

// RegisterHostedPlugin remembers only manifest-validated runtime evidence from a
// hello. The manifest, not the caller's namespace or directory name, is the key.
// This daemon-owned file is independent of session records and config overrides.
func RegisterHostedPlugin(dir, rsrc string) error {
	if rsrc == "" {
		return nil
	}
	root, err := filepath.Abs(filepath.Dir(filepath.Clean(rsrc)))
	if err != nil {
		return err
	}
	name, err := bundledPluginName(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, hostedPluginRootsFile)
	lock := flock.New(path + ".lock")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, 20*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("timed out locking hosted plugin roots")
	}
	defer lock.Unlock() //nolint:errcheck
	roots, err := readHostedPluginRoots(dir)
	if err != nil {
		return err
	}
	if roots[name] == root {
		return nil
	}
	roots[name] = root
	raw, err := json.MarshalIndent(roots, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(raw, '\n'), 0o600)
}

func readHostedPluginRoots(dir string) (map[string]string, error) {
	roots := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(dir, hostedPluginRootsFile))
	if os.IsNotExist(err) {
		return roots, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &roots); err != nil {
		return nil, err
	}
	if roots == nil {
		roots = map[string]string{}
	}
	return roots, nil
}

// HostedPluginEnvironment supplies the package-owned policy that the Python
// launcher normally receives from its plugin MCP entry. Blank values clear
// starter policy for knobs the selected package does not declare.
func HostedPluginEnvironment(plugin string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(plugin, ".claude-plugin", "plugin.json"))
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
	env := map[string]string{
		"WS_RSRC_ROOT":     filepath.Join(plugin, "rsrc"),
		"WS_SKILLS_ROOT":   filepath.Join(plugin, "skills"),
		"WS_MCP_NAMESPACE": "", "WS_MCP_SETUP_TOOL": "", "WS_MCP_NO_AGENT": "",
		"WS_MCP_CLI_NAME": "", "WS_MCP_CONFIG_MANIFESTS": "",
	}
	for key, value := range manifest.Servers[manifest.Name].Env {
		env[key] = strings.ReplaceAll(value, "${CLAUDE_PLUGIN_ROOT}", plugin)
	}
	return env, nil
}

// ResolveHostedPlugin reads the daemon's global setting at each process start.
// Registry entries are revalidated: removed or replaced installs fall back to
// the starter's own bundle with a visible warning, never a missing-package
// launch refusal. No connection's config home participates in this decision.
func ResolveHostedPlugin() (root, warning string, err error) {
	resolver := wsconfig.NewResolver(wsconfig.Options{}, map[string]string{wsconfig.HostedPluginPackageKey: "wsflow"}, nil, nil)
	selected, configErr := resolver.Get("", wsconfig.HostedPluginPackageKey)
	name := selected.Value
	if name != "ws" && name != "wsflow" {
		name = "wsflow"
	}
	dir, registryErr := wsdashboard.Dir("")
	if registryErr == nil && configErr == nil {
		var roots map[string]string
		roots, registryErr = readHostedPluginRoots(dir)
		if registryErr == nil {
			if candidate := roots[name]; filepath.IsAbs(candidate) {
				if actual, e := bundledPluginName(candidate); e == nil && actual == name {
					return candidate, "", nil
				}
			}
		}
	}
	root, err = daemonPluginRoot()
	if err != nil {
		return "", "", err
	}
	fallback, _ := bundledPluginName(root)
	if fallback == name && configErr == nil {
		return root, "", nil
	}
	reason := "selected package root is unavailable"
	if configErr != nil {
		reason = fmt.Sprintf("cannot read global package setting: %v", configErr)
	} else if registryErr != nil {
		reason = fmt.Sprintf("cannot read package registry: %v", registryErr)
	}
	return root, fmt.Sprintf("Hosted plugin %s: %s; using daemon package %s at %s instead.", name, reason, fallback, root), nil
}
