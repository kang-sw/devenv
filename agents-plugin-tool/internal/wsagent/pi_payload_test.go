package wsagent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func copyPiOwner(t *testing.T, source, dest string) {
	t.Helper()
	for _, name := range []string{"native-pi", "rsrc", "skills", "bin/ws-pi-payload.py", "bin/ws-mcp-launcher.py", "runtime.json", "dashboard-config-manifest.json", ".claude-plugin/plugin.json"} {
		from := filepath.Join(source, filepath.FromSlash(name))
		err := filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(source, path)
			raw, err := os.ReadFile(path)
			if err == nil {
				piLeadFile(t, filepath.Join(dest, rel), string(raw))
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Recreate the producer's complete input inventory, not just a sibling manifest:
// local-first is valid only while the prepared payload matches these sources.
func copyPiSourceInputs(t *testing.T, owner, repo string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(owner, "native-pi", "first-party-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Inputs map[string]string `json:"inputs"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for rel := range manifest.Inputs {
		input, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		piLeadFile(t, filepath.Join(repo, filepath.FromSlash(rel)), string(input))
	}
	sibling := filepath.Join(repo, "agents-plugin-pi")
	piLeadDeps(t, sibling)
	return sibling
}

func TestPiPreparedPackageOnlyLayouts(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python 3 unavailable")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(base, "config"))
	for _, product := range []string{"ws", "wsflow"} {
		for _, layout := range []string{".claude/plugins/cache/vendor", ".codex/plugins/cache/vendor", "shell-snapshot", "windows-snapshot"} {
			t.Run(product+"/"+layout, func(t *testing.T) {
				source := filepath.Join("..", "..", "..", "agents-plugin")
				if product == "wsflow" {
					source += "-wsflow"
				}
				owner := filepath.Join(base, filepath.FromSlash(layout), product, "0.50.0")
				copyPiOwner(t, source, owner)
				t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(owner, "rsrc"))
				entries, err := piBundledEntries()
				if err != nil || len(entries) != 1 {
					t.Fatalf("package-only native entries: %v, %v", entries, err)
				}
				root := filepath.Dir(filepath.Dir(entries[0]))
				if err := piBundleFresh(root); err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(root, filepath.Join(base, "cache", "native-pi")) || !strings.HasSuffix(entries[0], filepath.FromSlash(piBundleFile)) {
					t.Fatalf("not a prepared native entry: %v", entries)
				}
				if got, err := piBundledEntries(); err != nil || !slices.Equal(got, entries) {
					t.Fatalf("unchanged second consumption: %v, %v", got, err)
				}
				// A valid source sibling must not displace the package-local payload.
				copyPiSourceInputs(t, owner, filepath.Dir(owner))
				if got, err := piBundledEntries(); err != nil || !slices.Equal(got, entries) {
					t.Fatalf("local-first selection: %v, %v", got, err)
				}
				for _, isolation := range []string{"default", "isolated"} {
					spec := StartSpec{Root: base, Cwd: base, Dir: filepath.Join(base, "session"), Lead: &LeadSpec{PluginIsolation: isolation}}
					piLeadFile(t, filepath.Join(base, "host", "package.json"), `{"name":"@earendil-works/pi-coding-agent"}`)
					cli := filepath.Join(base, "host", "dist", "cli.js")
					piLeadFile(t, cli, "")
					piLeadFile(t, filepath.Join(filepath.Dir(cli), "index.js"), "")
					warning := "stale"
					spec.Lead.PluginWarning = func(text string) { warning = text }
					_, args, err := piLeadLaunch("node", []string{cli}, spec)
					if err != nil || warning != "" || !slices.Contains(args, entries[0]) {
						t.Fatalf("%s native launch: %v, warning %q, error %v", isolation, args, warning, err)
					}
				}
				// Invalid local output takes the visible bridge path, never the sibling.
				piLeadFile(t, filepath.Join(owner, "native-pi", "closure.zip"), "broken")
				spec := StartSpec{Root: base, Cwd: base, Dir: filepath.Join(base, "fallback"), Lead: &LeadSpec{PluginIsolation: "isolated", Env: map[string]string{EnvHostedSessionKey: "test-key"}}}
				warning := ""
				spec.Lead.PluginWarning = func(text string) { warning = text }
				spec.Lead.WSMCP = func() (MCPCommand, error) { return MCPCommand{Command: "test-only"}, nil }
				_, args, err := piLeadLaunch("pi", nil, spec)
				if err != nil || !strings.Contains(warning, "using the ws MCP bridge") || !slices.Contains(args, "builtin:mcp") {
					t.Fatalf("corrupt prepared fallback: %v, %q, %v", args, warning, err)
				}
			})
		}
	}
}
