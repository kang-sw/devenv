package wsagent

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func TestPiHostedPackageRegistrySettingMatrix(t *testing.T) {
	dir := hostedPluginHomes(t)
	base := t.TempDir()
	roots := map[string]string{}
	for _, owner := range []string{"ws", "wsflow"} {
		root := filepath.Join(base, owner)
		roots[owner] = root
		noAgent := ""
		if owner == "wsflow" {
			noAgent = "1"
		}
		piLeadFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"), fmt.Sprintf(`{"name":%q,"version":"0.50.1","mcpServers":{%q:{"env":{"WS_MCP_NAMESPACE":%q,"WS_MCP_NO_AGENT":%q,"WS_MCP_CONFIG_MANIFESTS":"${CLAUDE_PLUGIN_ROOT}/config-manifest.json;${CLAUDE_PLUGIN_ROOT}/dashboard-config-manifest.json"}}}}`, owner, owner, owner, noAgent))
		if err := RegisterHostedPlugin(dir, filepath.Join(root, "rsrc"), hostedPluginTestVersion); err != nil {
			t.Fatal(err)
		}
	}
	payload := filepath.Join(base, "agents-plugin-pi")
	entry := piLeadBundle(t, payload)
	usePreparedPiFixture(t, base, payload)
	for _, starter := range []string{"ws", "wsflow"} {
		for _, setting := range []string{"", "ws", "wsflow"} {
			t.Run(starter+"/"+setting, func(t *testing.T) {
				t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(roots[starter], "rsrc"))
				if setting == "" {
					r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
					if err := r.Unset(wsconfig.HostedPluginPackageKey, wsconfig.SetOptions{}); err != nil {
						t.Fatal(err)
					}
				} else {
					setHostedPackage(t, setting)
				}
				want := setting
				if want == "" {
					want = "wsflow"
				}
				spec := StartSpec{Root: base, Dir: t.TempDir(), Lead: &LeadSpec{DaemonVersion: hostedPluginTestVersion, PluginIsolation: "isolated"}}
				piLeadFile(t, filepath.Join(piDir(spec.Dir), ".keep"), "")
				warning := "stale"
				spec.Lead.PluginWarning = func(text string) { warning = text }
				_, args, err := piLeadLaunch("pi", nil, spec)
				if err != nil || !slices.Contains(args, entry) || warning != "" {
					t.Fatalf("launch args %v, warning %q, error %v", args, warning, err)
				}
				if got := spec.Lead.Env["WS_PI_WORKFLOW_ROOT"]; got != roots[want] {
					t.Fatalf("selected root %q, want %q", got, roots[want])
				}
				if got := spec.Lead.Env["WS_MCP_NAMESPACE"]; got != want {
					t.Fatalf("namespace %q, want %q", got, want)
				}
				manifests := filepath.Join(roots[want], "config-manifest.json") + ";" + filepath.Join(roots[want], "dashboard-config-manifest.json")
				if got := spec.Lead.Env["WS_MCP_CONFIG_MANIFESTS"]; got != manifests {
					t.Fatalf("manifest policy %q, want %q", got, manifests)
				}
			})
		}
	}
}
