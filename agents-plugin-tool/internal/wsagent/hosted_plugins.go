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

// hostedPluginPackages are the packages a hosted lead's bundle may be
// (ResolveHostedPlugin).
var hostedPluginPackages = []string{"ws", "wsflow"}

// RegisterHostedPlugin remembers only manifest-validated runtime evidence from a
// hello. The manifest, not the caller's namespace or directory name, is the key.
// Equal/newer hellos win; an older caller cannot downgrade a usable stored root.
// This daemon-owned file is independent of session records and config overrides.
func RegisterHostedPlugin(dir, rsrc, helloVersion string) error {
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
	if storedRoot := roots[name]; filepath.IsAbs(storedRoot) {
		if stored, err := readBundledPluginManifest(storedRoot); err == nil && stored.Name == name &&
			wsdashboard.CompareVersions(helloVersion, stored.Version) < 0 {
			return nil
		}
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
		if strings.Contains(value, "${CLAUDE_PLUGIN_ROOT}") {
			// Plugin-root templates are filesystem paths (possibly a ';'
			// list). Normalize their literal suffixes before substituting the
			// native root; leave unrelated env strings such as URLs intact.
			value = strings.ReplaceAll(filepath.FromSlash(value), "${CLAUDE_PLUGIN_ROOT}", plugin)
		}
		env[key] = value
	}
	return env, nil
}

// ResolveHostedPlugin reads the daemon's global setting at each process start.
// Registry entries are revalidated: removed or replaced installs fall back to
// the starter's own bundle with a visible warning, never a missing-package
// launch refusal. No connection's config home participates in this decision.
func ResolveHostedPlugin(daemonVersion string) (root, warning string, err error) {
	resolver := wsconfig.NewResolver(wsconfig.Options{}, map[string]string{wsconfig.HostedPluginPackageKey: "wsflow"}, nil, nil)
	selected, configErr := resolver.Get("", wsconfig.HostedPluginPackageKey)
	name := selected.Value
	if name != "ws" && name != "wsflow" {
		if configErr == nil {
			warning = fmt.Sprintf("invalid %s %q; using wsflow.", wsconfig.HostedPluginPackageKey, name)
		}
		name = "wsflow"
	}
	// The starter's own selected package is authoritative. Connecting older
	// sessions must not switch it to another installed patch of that package.
	root, ownErr := daemonPluginRoot()
	var fallback string
	if ownErr == nil {
		fallback, _ = bundledPluginName(root)
	}
	if ownErr == nil && fallback == name && configErr == nil {
		return root, warning, nil
	}
	reason := "selected package root is unavailable"
	dir, registryErr := wsdashboard.Dir("")
	if registryErr == nil && configErr == nil {
		var roots map[string]string
		roots, registryErr = readHostedPluginRoots(dir)
		if registryErr == nil {
			if candidate := roots[name]; filepath.IsAbs(candidate) {
				if manifest, e := readBundledPluginManifest(candidate); e == nil && manifest.Name == name {
					if wsdashboard.SameMinor(manifest.Version, daemonVersion) {
						return candidate, warning, nil
					}
					reason = fmt.Sprintf("selected package version %q does not match daemon version %q's minor", manifest.Version, daemonVersion)
				}
			}
		}
	}
	if ownErr != nil {
		return "", "", ownErr
	}
	if configErr != nil {
		reason = fmt.Sprintf("cannot read global package setting: %v", configErr)
	} else if registryErr != nil {
		reason = fmt.Sprintf("cannot read package registry: %v", registryErr)
	}
	fallbackWarning := fmt.Sprintf("Hosted plugin %s: %s; using daemon package %s at %s instead.", name, reason, fallback, root)
	if warning != "" {
		fallbackWarning = warning + " " + fallbackWarning
	}
	return root, fallbackWarning, nil
}
