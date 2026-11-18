package wsagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestPiHostedEmbeddedPackageMatrix(t *testing.T) {
	dir := hostedPluginHomes(t)
	if os.Getenv("WS_TEST_EMBEDDED_PACKAGES") == "1" {
		t.Setenv("WS_RSRC_DEV_CHECKOUT", "")
	}
	t.Setenv("WS_RSRC_ROOT", filepath.Join(t.TempDir(), "missing", "rsrc"))
	writeLeadPluginFixture(t, filepath.Join(dir, "hosted-plugin-roots.json"), `{stale registry`)
	for _, setting := range []string{"", "ws", "wsflow"} {
		t.Run(setting, func(t *testing.T) {
			r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
			if setting == "" {
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
			spec := StartSpec{Root: t.TempDir(), Dir: t.TempDir(), Lead: &LeadSpec{PluginIsolation: "isolated"}}
			piLeadFile(t, filepath.Join(piDir(spec.Dir), ".keep"), "")
			warning := "stale"
			spec.Lead.PluginWarning = func(text string) { warning = text }
			_, args, err := piLeadLaunch("pi", nil, spec)
			if err != nil || warning != "" {
				t.Fatalf("launch: %v %q %v", args, warning, err)
			}
			workflow := spec.Lead.Env["WS_PI_WORKFLOW_ROOT"]
			if workflow == "" {
				t.Fatal("missing staged workflow owner")
			}
			var owner struct {
				Name string `json:"name"`
			}
			raw, err := os.ReadFile(filepath.Join(workflow, ".claude-plugin", "plugin.json"))
			if err != nil || json.Unmarshal(raw, &owner) != nil || owner.Name != want {
				t.Fatalf("workflow owner: %s %v", raw, err)
			}
			for _, path := range []string{"config-manifest.json", "dashboard-config-manifest.json", "rsrc/manifest.json", "skills/lead-run/SKILL.md"} {
				if _, err := os.Stat(filepath.Join(workflow, filepath.FromSlash(path))); err != nil {
					t.Fatal(err)
				}
			}
			policy, err := HostedPluginEnvironment(want, workflow)
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range policy {
				if spec.Lead.Env[key] != value {
					t.Fatalf("%s = %q want %q", key, spec.Lead.Env[key], value)
				}
			}
			// Workers prepare the real native extension using only the selected name.
			entries, err := piBundledEntries("ignored old version")
			if err != nil || len(entries) != 1 || !slices.Contains(args, entries[0]) {
				t.Fatalf("worker entries %v %v; lead %v", entries, err, args)
			}
			if !fileExists(entries[0]) {
				t.Fatal("worker entry missing")
			}
		})
	}
}
