package wsagent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wspackage"
)

const hostedPluginTestVersion = "0.50.1"

func hostedPluginHomes(t *testing.T) string {
	t.Helper()
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	dir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func setHostedPackage(t *testing.T, value string) {
	t.Helper()
	r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	if err := r.Set(wsconfig.HostedPluginPackageKey, value, wsconfig.SetOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestHostedWorkerMCPWarningPersists(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": `{"backend":"claude","ws_tools":["tickets.query"]}`})
	h.runner.Close()
	const warning = "invalid hosted package; using wsflow."
	h.config.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "launcher", PluginWarning: warning}, nil }
	h.start()
	id := h.create("chat").ID
	if _, err := h.runner.Send(id, "hello"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "worker start", func() bool { return h.backend.startCount() == 1 })
	rec := readEventually(t, "worker warning", func() (Record, error) { return readRecord(wsdashboard.SessionDir(h.dir, id)) })
	if rec.PluginWarning != warning || h.state(id).PluginWarning != warning {
		t.Fatal("warning lost")
	}
	h.runner.Close()
	h.start()
	if h.state(id).PluginWarning != warning {
		t.Fatal("restart lost warning")
	}
}

func TestHostedPackageSelectionIgnoresStarterAndStaleRegistry(t *testing.T) {
	dir := hostedPluginHomes(t)
	registry := filepath.Join(dir, "hosted-plugin-roots.json")
	writeLeadPluginFixture(t, registry, `{broken legacy registry`)
	for _, starter := range []string{"", filepath.Join(t.TempDir(), "missing", "rsrc")} {
		t.Setenv("WS_RSRC_ROOT", starter)
		for _, selected := range []string{"", "ws", "wsflow"} {
			r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
			if selected == "" {
				if err := r.Unset(wsconfig.HostedPluginPackageKey, wsconfig.SetOptions{}); err != nil {
					t.Fatal(err)
				}
			} else {
				setHostedPackage(t, selected)
			}
			want := selected
			if want == "" {
				want = "wsflow"
			}
			name, warning, err := ResolveHostedPackage()
			if err != nil || name != want || warning != "" {
				t.Fatalf("selected %q: %q %q %v", selected, name, warning, err)
			}
			args, err := claudeLeadPluginArgs(claudePluginLeadSpec(t))
			if err != nil || slices.Contains(args, "--plugin-dir") {
				t.Fatalf("Claude loads no plugin: %v %v", args, err)
			}
		}
	}
	if data, err := os.ReadFile(registry); err != nil || string(data) != `{broken legacy registry` {
		t.Fatal("legacy registry was touched")
	}
}

func TestHostedPackageInvalidSelectionWarns(t *testing.T) {
	hostedPluginHomes(t)
	// Bypass the config writer's enum validation, as a hand-edited config can.
	writeLeadPluginFixture(t, filepath.Join(os.Getenv("WS_CONFIG_HOME"), "config.json"), `{"overrides":{"hosted.plugin_package":"other"}}`)
	name, warning, err := ResolveHostedPackage()
	if err != nil || name != "wsflow" || !strings.Contains(warning, "using wsflow") {
		t.Fatalf("%q %q %v", name, warning, err)
	}
}

func TestHostedPackageSettingWaitsForProcessStart(t *testing.T) {
	dir := hostedPluginHomes(t)
	setHostedPackage(t, "ws")
	ws, err := wspackage.StageHosted("ws", dir)
	if err != nil {
		t.Fatal(err)
	}
	h := newLeadHarness(t).start()
	id, holder := h.runLead("first")
	if got := leadLauncherPlugin(t, h.holders.spawned(holder).Argv); got != ws {
		t.Fatalf("launcher = %q want %q", got, ws)
	}
	h.holders.out(holder, turnLines("session", "u1", "ok")...)
	h.idle(id)
	setHostedPackage(t, "wsflow")
	h.restart()
	if h.holder(id) != holder || leadLauncherPlugin(t, h.holders.spawned(holder).Argv) != ws {
		t.Fatal("reattach changed selection")
	}
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "next process")
	wsflow, err := wspackage.StageHosted("wsflow", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := leadLauncherPlugin(t, h.holders.spawned(h.holder(id)).Argv); got != wsflow {
		t.Fatalf("next launcher = %q want %q", got, wsflow)
	}
}
