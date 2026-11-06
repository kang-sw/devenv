package mcp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// Each package (ws, wsflow, Pi) ships a "dashboard."-namespace manifest beside
// its own manifest and lists both in WS_MCP_CONFIG_MANIFESTS. The three
// declare the same keys and differ only in dashboard.enabled's default.

var dashboardPackages = []struct {
	name      string
	own       string // the package's existing manifest
	dashboard string
	enabled   bool
}{
	{"ws", claudeManifestPath, filepath.Join("..", "..", "..", "agents-plugin", "dashboard-config-manifest.json"), true},
	{"wsflow", wsflowClaudeManifestPath, filepath.Join("..", "..", "..", "agents-plugin-wsflow", "dashboard-config-manifest.json"), false},
	{"pi", filepath.Join("..", "..", "..", "agents-plugin-pi", "config-manifest.json"), filepath.Join("..", "..", "..", "agents-plugin-pi", "dashboard-config-manifest.json"), true},
}

// dashboardServer builds a server whose launcher listed manifestList, with
// throwaway config and cache homes so no real tune leaks in.
func dashboardServer(t *testing.T, manifestList string) *Server {
	t.Helper()
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv(envConfigManifests, manifestList)
	return NewServer(t.TempDir(), "test")
}

func TestShippedDashboardManifestsLoadBesideEachPackageManifest(t *testing.T) {
	for _, pkg := range dashboardPackages {
		t.Run(pkg.name, func(t *testing.T) {
			// Launcher config files are the same on every platform, so they
			// join the two paths with ";", which every platform accepts.
			list := pkg.own + ";" + pkg.dashboard
			declared := loadDeclaredConfig(list)
			if len(declared.rejected) != 0 {
				t.Fatalf("rejected: %v", declared.rejected)
			}
			own := loadDeclaredConfig(pkg.own)
			if len(declared.keys) != len(own.keys)+2 {
				t.Fatalf("%d keys loaded, want the package's %d plus 2 dashboard keys", len(declared.keys), len(own.keys))
			}
			enabled, ok := declared.lookup(DashboardEnabledKey)
			if !ok || enabled.DefaultValue != pkg.enabled {
				t.Fatalf("%s = %+v (declared %v), want default %v", DashboardEnabledKey, enabled.DefaultValue, ok, pkg.enabled)
			}
			port, ok := declared.lookup(DashboardPortKey)
			if !ok || port.DefaultValue != int64(wsdashboard.DefaultPort) {
				t.Fatalf("%s = %+v (declared %v), want default %d", DashboardPortKey, port.DefaultValue, ok, wsdashboard.DefaultPort)
			}
			got := dashboardServer(t, list).DashboardSettings()
			if want := (DashboardSettings{Enabled: pkg.enabled, Port: wsdashboard.DefaultPort}); got != want {
				t.Fatalf("DashboardSettings = %+v, want %+v", got, want)
			}
		})
	}
}

func TestShippedDashboardManifestsDifferOnlyInTheEnabledDefault(t *testing.T) {
	ws := readManifestJSON(t, dashboardPackages[0].dashboard)
	wsKeys := manifestKeyObjects(t, dashboardPackages[0].dashboard, ws)
	for _, pkg := range dashboardPackages[1:] {
		other := readManifestJSON(t, pkg.dashboard)
		for _, field := range []string{"schema_version", "namespace"} {
			if !reflect.DeepEqual(ws[field], other[field]) {
				t.Fatalf("%s: %s differs from ws: %v vs %v", pkg.name, field, other[field], ws[field])
			}
		}
		otherKeys := manifestKeyObjects(t, pkg.dashboard, other)
		if len(otherKeys) != len(wsKeys) {
			t.Fatalf("%s declares %d keys, ws %d", pkg.name, len(otherKeys), len(wsKeys))
		}
		for i := range wsKeys {
			a, b := wsKeys[i], otherKeys[i]
			for field := range a {
				if field == "default" && a["key"] == DashboardEnabledKey {
					continue
				}
				if !reflect.DeepEqual(a[field], b[field]) {
					t.Errorf("%s %v: %s differs from ws: %v vs %v", pkg.name, a["key"], field, b[field], a[field])
				}
			}
			if len(a) != len(b) {
				t.Errorf("%s %v: field sets differ", pkg.name, a["key"])
			}
		}
	}
	for _, k := range wsKeys {
		if desc, _ := k["description"].(string); k["key"] == DashboardEnabledKey && strings.Contains(strings.ToLower(desc), "default") {
			t.Errorf("%s's default differs between packages, so its shared description must not mention a default", DashboardEnabledKey)
		}
	}
}

// TestDashboardOffWithoutALoadedDashboardManifest: a missing or unparsable
// dashboard manifest leaves the dashboard off, while the package's own
// manifest still loads and the server still builds.
func TestDashboardOffWithoutALoadedDashboardManifest(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "dashboard-config-manifest.json")
	if err := os.WriteFile(broken, []byte(`{"schema_version": 1, "namespace": "dashboard.", "keys": [`), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, list := range map[string]string{
		"missing":    claudeManifestPath + ";" + filepath.Join(t.TempDir(), "absent.json"),
		"unparsable": claudeManifestPath + ";" + broken,
		"unlisted":   claudeManifestPath,
		"empty":      "",
	} {
		t.Run(name, func(t *testing.T) {
			server := dashboardServer(t, list)
			if got := server.DashboardSettings(); got.Enabled || got.Port != wsdashboard.DefaultPort {
				t.Fatalf("DashboardSettings = %+v, want off with the default port", got)
			}
			if list != "" {
				if _, ok := server.declared.lookup("claude.status_band_enabled"); !ok {
					t.Fatal("the package's own manifest did not load beside the failed one")
				}
			}
		})
	}
}

func TestSplitManifestListAcceptsSemicolonOnEveryPlatform(t *testing.T) {
	got := splitManifestList("a.json;b.json" + string(os.PathListSeparator) + "c.json")
	if want := []string{"a.json", "b.json", "c.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("splitManifestList = %q, want %q", got, want)
	}
}

// TestDashboardSettingsReadTunedValues: a project- or global-scope tune
// reaches the startup read.
func TestDashboardSettingsReadTunedValues(t *testing.T) {
	list := dashboardPackages[0].own + ";" + dashboardPackages[0].dashboard
	server := dashboardServer(t, list)
	r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	if err := r.Set(DashboardEnabledKey, "false", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeProject}); err != nil {
		t.Fatal(err)
	}
	if err := r.Set(DashboardPortKey, "31999", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	if got, want := server.DashboardSettings(), (DashboardSettings{Enabled: false, Port: 31999}); got != want {
		t.Fatalf("DashboardSettings = %+v, want %+v", got, want)
	}
	// An invalid stored port falls back to the manifest default.
	if err := r.Set(DashboardPortKey, "not-a-port", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeProject}); err != nil {
		t.Fatal(err)
	}
	if got := server.DashboardSettings(); got.Port != wsdashboard.DefaultPort {
		t.Fatalf("port with an invalid stored value = %d, want %d", got.Port, wsdashboard.DefaultPort)
	}
}
