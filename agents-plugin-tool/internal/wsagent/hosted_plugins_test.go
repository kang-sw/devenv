package wsagent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsrsrc"
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
	const warning = "Hosted plugin wsflow unavailable; using daemon package ws instead."
	h.config.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "launcher", PluginWarning: warning}, nil }
	h.start()
	id := h.create("chat").ID
	if _, err := h.runner.Send(id, "hello"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "worker start", func() bool { return h.backend.startCount() == 1 })
	rec := readEventually(t, "worker warning", func() (Record, error) { return readRecord(wsdashboard.SessionDir(h.dir, id)) })
	if rec.PluginWarning != warning || h.state(id).PluginWarning != warning {
		t.Fatalf("worker warning = %q, state %q", rec.PluginWarning, h.state(id).PluginWarning)
	}
	h.runner.Close()
	h.start()
	if h.state(id).PluginWarning != warning {
		t.Fatal("worker restart lost fallback warning")
	}
}

func TestHostedPluginSelectionIndependentOfStarter(t *testing.T) {
	dir := hostedPluginHomes(t)
	roots := map[string]string{}
	for _, name := range []string{"ws", "wsflow"} {
		roots[name] = leadBundledFixture(t, name)
		if err := RegisterHostedPlugin(dir, filepath.Join(roots[name], "rsrc"), hostedPluginTestVersion); err != nil {
			t.Fatal(err)
		}
	}
	for _, starter := range []string{"ws", "wsflow"} {
		t.Run(starter, func(t *testing.T) {
			t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(roots[starter], "rsrc"))
			for _, selected := range []string{"", "ws", "wsflow", "ws"} {
				if selected == "" {
					r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
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
				// Each call creates fresh readers, as after daemon replacement.
				got, warning, err := ResolveHostedPlugin(hostedPluginTestVersion)
				if err != nil || got != roots[want] || warning != "" {
					t.Fatalf("selected %q: %q, %q, %v", selected, got, warning, err)
				}
				// A Claude lead loads no package whichever is selected: the
				// selection reaches it as its ws-mcp's launcher.
				spec := claudePluginLeadSpec(t)
				launchWarning := "unreported"
				spec.Lead.PluginWarning = func(text string) { launchWarning = text }
				args, err := claudeLeadPluginArgs(spec)
				if err != nil || slices.Contains(args, "--plugin-dir") || launchWarning != "" {
					t.Fatalf("Claude selected %q: %v, %q, %v", selected, args, launchWarning, err)
				}
			}
		})
	}
}

func TestHostedPluginRegistryRejectsInvalidAndMismatchedRoots(t *testing.T) {
	dir := hostedPluginHomes(t)
	ws := leadBundledFixture(t, "ws")
	if err := RegisterHostedPlugin(dir, filepath.Join(ws, "rsrc"), hostedPluginTestVersion); err != nil {
		t.Fatal(err)
	}
	bad := leadBundledFixture(t, "other")
	if err := RegisterHostedPlugin(dir, filepath.Join(bad, "rsrc"), hostedPluginTestVersion); err == nil {
		t.Fatal("registered unsupported manifest")
	}
	roots, err := readHostedPluginRoots(dir)
	if err != nil || len(roots) != 1 || roots["ws"] != ws {
		t.Fatalf("registry = %v, %v", roots, err)
	}
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(ws, "rsrc"))
	// A forged or stale association cannot make a ws manifest serve as wsflow.
	writeLeadPluginFixture(t, filepath.Join(dir, hostedPluginRootsFile), `{"wsflow":"`+filepath.ToSlash(ws)+`"}`)
	root, warning, err := ResolveHostedPlugin(hostedPluginTestVersion)
	if err != nil || root != ws || !strings.Contains(warning, "wsflow") || !strings.Contains(warning, "daemon package ws at ") {
		t.Fatalf("mismatch fallback = %q, %q, %v", root, warning, err)
	}
}

func TestHostedPluginFallbackMissingAndStale(t *testing.T) {
	dir := hostedPluginHomes(t)
	wsflow := leadBundledFixture(t, "wsflow")
	ws := leadBundledFixture(t, "ws")
	for _, state := range []string{"unregistered", "removed", "renamed", "corrupt registry"} {
		t.Run(state, func(t *testing.T) {
			t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(ws, "rsrc"))
			if state != "unregistered" {
				writeLeadPluginFixture(t, filepath.Join(wsflow, ".claude-plugin", "plugin.json"), `{"name":"wsflow"}`)
				if err := RegisterHostedPlugin(dir, filepath.Join(wsflow, "rsrc"), hostedPluginTestVersion); err != nil {
					t.Fatal(err)
				}
			}
			switch state {
			case "removed":
				if err := os.Remove(filepath.Join(wsflow, ".claude-plugin", "plugin.json")); err != nil {
					t.Fatal(err)
				}
			case "renamed":
				writeLeadPluginFixture(t, filepath.Join(wsflow, ".claude-plugin", "plugin.json"), `{"name":"ws"}`)
			case "corrupt registry":
				writeLeadPluginFixture(t, filepath.Join(dir, hostedPluginRootsFile), `{`)
			}
			root, warning, err := ResolveHostedPlugin(hostedPluginTestVersion)
			if err != nil || root != ws || !strings.Contains(warning, "wsflow") || !strings.Contains(warning, "daemon package ws at ") {
				t.Fatalf("fallback = %q, %q, %v", root, warning, err)
			}
		})
	}
}

func TestHostedPluginWarningPersistsAndSettingWaitsForProcessStart(t *testing.T) {
	dir := hostedPluginHomes(t)
	ws := leadBundledFixture(t, "ws")
	h := newLeadHarness(t).start()
	id, holder := h.runLead("first")
	warning := h.record(id).PluginWarning
	if !strings.Contains(warning, "wsflow") || !strings.Contains(warning, "daemon package ws at ") || h.state(id).PluginWarning != warning {
		t.Fatalf("warning not persisted/exposed: %q", warning)
	}
	if got := leadLauncherPlugin(t, h.holders.spawned(holder).Argv); got != ws {
		t.Fatalf("fallback plugin = %q", got)
	}
	if st := h.state(id); !st.Process || st.Error != "" {
		t.Fatalf("usable fallback must warn without a launch error: %+v", st)
	}
	h.holders.out(holder, turnLines("session", "u1", "ok")...)
	h.idle(id)
	wsflow := leadBundledFixture(t, "wsflow")
	if err := RegisterHostedPlugin(dir, filepath.Join(wsflow, "rsrc"), hostedPluginTestVersion); err != nil {
		t.Fatal(err)
	}
	setHostedPackage(t, "ws")
	if !h.state(id).Process || h.holder(id) != holder || h.record(id).PluginWarning != warning {
		t.Fatal("changing the setting affected the running process")
	}
	setHostedPackage(t, "wsflow")
	h.restart()
	if got := h.holder(id); got != holder {
		t.Fatalf("reattach replaced holder: %q -> %q", holder, got)
	}
	if got := leadLauncherPlugin(t, h.holders.spawned(holder).Argv); got != ws {
		t.Fatalf("reattach changed plugin to %q", got)
	}
	if got := h.record(id).PluginWarning; got != warning {
		t.Fatalf("reattach changed warning: %q", got)
	}
	if st := h.state(id); st.Error != "" {
		t.Fatalf("reattached usable fallback must not be an error: %+v", st)
	}
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "next process")
	next := h.holder(id)
	if got := leadLauncherPlugin(t, h.holders.spawned(next).Argv); got != wsflow {
		t.Fatalf("next start plugin = %q, want %q", got, wsflow)
	}
	if got := h.record(id).PluginWarning; got != "" {
		t.Fatalf("next successful selection kept stale warning: %q", got)
	}
}
