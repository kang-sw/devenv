package wsagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func hostedVersionFixture(t *testing.T, name, version string) string {
	t.Helper()
	root := t.TempDir()
	raw, err := json.Marshal(map[string]string{"name": name, "version": version})
	if err != nil {
		t.Fatal(err)
	}
	writeLeadPluginFixture(t, filepath.Join(root, ".claude-plugin", "plugin.json"), string(raw))
	if err := os.MkdirAll(filepath.Join(root, "rsrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHostedPluginRegistryDoesNotDowngrade(t *testing.T) {
	dir := hostedPluginHomes(t)
	first := hostedVersionFixture(t, "ws", "0.50.2")
	if err := RegisterHostedPlugin(dir, filepath.Join(first, "rsrc"), "0.50.2"); err != nil {
		t.Fatal(err)
	}
	want := first
	for _, tc := range []struct {
		name, manifestVersion, helloVersion string
		replace                             bool
	}{
		{"older patch", "0.50.1", "0.50.1", false},
		{"older minor", "0.49.9", "0.49.9", false},
		{"unversioned hello", "0.50.1", "", false},
		{"equal version last seen wins", "0.50.2", "0.50.2", true},
		{"newer patch last seen wins", "0.50.3", "0.50.3", true},
		{"later older hello", "0.50.2", "0.50.2", false},
		{"newer minor last seen wins", "0.51.0", "0.51.0", true},
	} {
		root := hostedVersionFixture(t, "ws", tc.manifestVersion)
		t.Run(tc.name, func(t *testing.T) {
			if err := RegisterHostedPlugin(dir, filepath.Join(root, "rsrc"), tc.helloVersion); err != nil {
				t.Fatal(err)
			}
			if tc.replace {
				want = root
			}
			roots, err := readHostedPluginRoots(dir)
			if err != nil || roots["ws"] != want {
				t.Fatalf("registry root = %q, %v; want %q", roots["ws"], err, want)
			}
		})
	}
}

func TestHostedPluginOwnSelectedRootPreferred(t *testing.T) {
	dir := hostedPluginHomes(t)
	own := hostedVersionFixture(t, "ws", "0.50.2")
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(own, "rsrc"))
	setHostedPackage(t, "ws")
	for _, version := range []string{"0.50.1", "0.50.3", "0.51.0"} {
		registered := hostedVersionFixture(t, "ws", version)
		if err := RegisterHostedPlugin(dir, filepath.Join(registered, "rsrc"), version); err != nil {
			t.Fatal(err)
		}
		root, warning, err := ResolveHostedPlugin("0.50.2")
		if err != nil || root != own || warning != "" {
			t.Fatalf("registered %s displaced daemon root: %q, %q, %v", version, root, warning, err)
		}
	}
	writeLeadPluginFixture(t, filepath.Join(dir, hostedPluginRootsFile), `{`)
	if root, warning, err := ResolveHostedPlugin("0.50.2"); err != nil || root != own || warning != "" {
		t.Fatalf("own selected root consulted corrupt registry: %q %q %v", root, warning, err)
	}
}

func TestHostedPluginRegistryVersionSkewFallsBack(t *testing.T) {
	for _, registeredVersion := range []string{"0.50.2", "0.52.0", "", "invalid"} {
		t.Run(registeredVersion, func(t *testing.T) {
			dir := hostedPluginHomes(t)
			own := hostedVersionFixture(t, "wsflow", "0.51.0")
			selected := hostedVersionFixture(t, "ws", registeredVersion)
			t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(own, "rsrc"))
			setHostedPackage(t, "ws")
			if err := RegisterHostedPlugin(dir, filepath.Join(selected, "rsrc"), registeredVersion); err != nil {
				t.Fatal(err)
			}
			root, warning, err := ResolveHostedPlugin("0.51.0")
			if err != nil || root != own || !strings.Contains(warning, "selected package version "+strconv.Quote(registeredVersion)) || !strings.Contains(warning, `daemon version "0.51.0"`) || !strings.Contains(warning, "daemon package wsflow at ") {
				t.Fatalf("version-skew fallback = %q %q %v", root, warning, err)
			}
			if registeredVersion == "0.50.2" {
				// The unchanged registry remains useful to a same-minor reader.
				if root, warning, err := ResolveHostedPlugin("0.50.3"); err != nil || root != selected || warning != "" {
					t.Fatalf("same-minor registry root rejected: %q %q %v", root, warning, err)
				}
			}
		})
	}
}

func TestHostedPluginVersionFallbackWarningPersists(t *testing.T) {
	dir := hostedPluginHomes(t)
	own := hostedVersionFixture(t, "wsflow", "0.51.0")
	selected := hostedVersionFixture(t, "ws", "0.50.2")
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(own, "rsrc"))
	setHostedPackage(t, "ws")
	if err := RegisterHostedPlugin(dir, filepath.Join(selected, "rsrc"), "0.50.2"); err != nil {
		t.Fatal(err)
	}
	h := newLeadHarness(t)
	h.opts.DaemonVersion = "0.51.0"
	h.opts.WSMCP = hostedLeadLauncher("0.51.0")
	h.start()
	id, holder := h.runLead("start with compatible fallback")
	if got := leadLauncherPlugin(t, h.holders.spawned(holder).Argv); got != own {
		t.Fatalf("launched plugin %q, want fallback %q", got, own)
	}
	warning := h.record(id).PluginWarning
	if !strings.Contains(warning, `"0.50.2"`) || !strings.Contains(warning, `"0.51.0"`) || h.state(id).PluginWarning != warning {
		t.Fatalf("missing persisted version warning: %q", warning)
	}
}

func TestHostedPluginInvalidSelectionWarns(t *testing.T) {
	for _, mode := range []string{"own default", "registered default", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			dir := hostedPluginHomes(t)
			ownName := "ws"
			if mode == "own default" {
				ownName = "wsflow"
			}
			own := hostedVersionFixture(t, ownName, "0.50.1")
			want := own
			if mode == "registered default" {
				want = hostedVersionFixture(t, "wsflow", "0.50.1")
				if err := RegisterHostedPlugin(dir, filepath.Join(want, "rsrc"), "0.50.1"); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(own, "rsrc"))
			setHostedPackage(t, "WS") // Models an invalid hand-edited global override.
			root, warning, err := ResolveHostedPlugin("0.50.1")
			if err != nil || root != want || !strings.Contains(warning, `invalid hosted.plugin_package "WS"; using wsflow`) {
				t.Fatalf("invalid selection = %q, %q, %v", root, warning, err)
			}
			if mode == "fallback" && !strings.Contains(warning, "daemon package ws at ") {
				t.Fatalf("invalid selection lost fallback warning: %q", warning)
			}
		})
	}
}
