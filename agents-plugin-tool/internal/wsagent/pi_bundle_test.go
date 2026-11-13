package wsagent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The committed bundle is generated on a machine with Node; the release host
// has none, so this test is the staleness gate there.
func TestPiBundleCommittedIsFresh(t *testing.T) {
	pkg := filepath.Join("..", "..", "..", "agents-plugin-pi")
	if err := piBundleFresh(pkg); err != nil {
		t.Fatalf("agents-plugin-pi bundle: %v.\nRegenerate it on a machine with Node and commit the result:\n  cd agents-plugin-pi && %s", err, piBundleRebuild)
	}
	if !fileExists(filepath.Join(pkg, "dist", "eff_large_wordlist.txt")) {
		t.Fatalf("agents-plugin-pi/dist/eff_large_wordlist.txt is missing; run: cd agents-plugin-pi && %s", piBundleRebuild)
	}
}

// piBundlePackage writes a source-shaped Pi package and, when fresh, a bundle
// whose header matches its inputs.
func piBundlePackage(t *testing.T, pkg string, fresh bool) string {
	t.Helper()
	piLeadFile(t, filepath.Join(pkg, "package.json"), `{"pi":{"extensions":["./src/index.ts"]}}`)
	piLeadFile(t, filepath.Join(pkg, "src", "index.ts"), `import "./util.ts";`)
	piLeadFile(t, filepath.Join(pkg, "src", "util.ts"), "export {};")
	piLeadFile(t, filepath.Join(pkg, "src", "helper.mjs"), "export {};")
	piLeadFile(t, filepath.Join(pkg, "src", "eff_large_wordlist.txt"), "11111\tabacus\n")
	piLeadFile(t, filepath.Join(pkg, "package-lock.json"), "{}")
	piLeadFile(t, filepath.Join(pkg, "scripts", "bundle.mjs"), "// bundler")
	hash, err := piBundleInputHash(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh {
		hash = strings.Repeat("0", len(hash))
	}
	bundle := filepath.Join(pkg, filepath.FromSlash(piBundleFile))
	piLeadFile(t, bundle, piBundleHeader+hash+"\nexport default function () {}\n")
	return bundle
}

func TestPiBundleInputHashCoversInputsOnly(t *testing.T) {
	pkg := t.TempDir()
	piBundlePackage(t, pkg, true)
	base, err := piBundleInputHash(pkg)
	if err != nil {
		t.Fatal(err)
	}
	for _, unrelated := range []string{"test/util.test.ts", "src/notes.md", "README.md", "dist/index.js"} {
		piLeadFile(t, filepath.Join(pkg, filepath.FromSlash(unrelated)), "changed")
		if got, _ := piBundleInputHash(pkg); got != base {
			t.Fatalf("%s is not a bundle input but changed the hash", unrelated)
		}
	}
	for _, input := range []string{"src/util.ts", "src/nested/deep.ts", "src/helper.mjs", "src/eff_large_wordlist.txt", "package-lock.json", "scripts/bundle.mjs"} {
		piLeadFile(t, filepath.Join(pkg, filepath.FromSlash(input)), "changed "+input)
		got, err := piBundleInputHash(pkg)
		if err != nil || got == base {
			t.Fatalf("editing %s must change the hash (%v)", input, err)
		}
		base = got
	}
}

func TestPiLeadPrefersFreshBundle(t *testing.T) {
	_, root := piLeadFixture(t)
	bundle := piBundlePackage(t, root, true)
	// No node_modules anywhere: the bundle inlines its deps.
	entries, err := piBundledEntries()
	if err != nil || !slices.Equal(entries, []string{bundle}) {
		t.Fatalf("fresh bundle entries = %v, %v", entries, err)
	}
	// Editing a source without regenerating must not run the old bundle.
	piLeadFile(t, filepath.Join(root, "src", "util.ts"), "export const changed = 1;")
	if entries, err := piBundledEntries(); err == nil || !strings.Contains(err.Error(), "stale") || !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("stale bundle without deps = %v, %v", entries, err)
	}
	piLeadDeps(t, filepath.Dir(root))
	if entries, err := piBundledEntries(); err != nil || !slices.Equal(entries, []string{filepath.Join(root, "src", "index.ts")}) {
		t.Fatalf("stale bundle with deps = %v, %v", entries, err)
	}
	if err := os.Remove(bundle); err != nil {
		t.Fatal(err)
	}
	if entries, err := piBundledEntries(); err != nil || !slices.Equal(entries, []string{filepath.Join(root, "src", "index.ts")}) {
		t.Fatalf("missing bundle with deps = %v, %v", entries, err)
	}
}

func TestPiLeadMissingDepsFallsBackToBridge(t *testing.T) {
	for _, isolation := range []string{"isolated", "default"} {
		t.Run(isolation, func(t *testing.T) {
			spec, root := piLeadFixture(t)
			piLeadFile(t, filepath.Join(root, "package.json"), `{"pi":{"extensions":["./src/index.ts"]}}`)
			piLeadFile(t, filepath.Join(root, "src", "index.ts"), `import "yaml";`)
			piLeadFile(t, filepath.Join(spec.Root, "node_modules", "yaml", "package.json"), `{"name":"yaml"}`)
			spec.Lead.PluginIsolation = isolation
			spec.Lead.Env = map[string]string{EnvHostedSessionKey: "lead-key"}
			warning := ""
			spec.Lead.PluginWarning = func(text string) { warning = text }
			spec.Lead.WSMCP = func() (MCPCommand, error) { return MCPCommand{Command: "/bundled/ws", Args: []string{"mcp"}}, nil }
			command, pre := "pi", []string(nil)
			if isolation == "default" {
				pkg := filepath.Join(spec.Root, "host-pi")
				cli := filepath.Join(pkg, "dist", "cli.js")
				piLeadFile(t, filepath.Join(pkg, "package.json"), `{"name":"@earendil-works/pi-coding-agent"}`)
				piLeadFile(t, cli, "")
				piLeadFile(t, filepath.Join(pkg, "dist", "index.js"), "")
				command, pre = "host-node", []string{cli}
			}
			_, args, err := piLeadLaunch(command, pre, spec)
			if err != nil {
				t.Fatal(err)
			}
			bridge := filepath.Join(piDir(spec.Dir), piBridgeFile)
			if !slices.Equal(args[len(args)-5:], []string{"--no-extensions", "--extension", "builtin:mcp", "--extension", bridge}) {
				t.Fatalf("missing deps must launch the bridge, got %v", args)
			}
			if !strings.Contains(warning, "using the ws MCP bridge") || !strings.Contains(warning, "linkedom, ipaddr.js") || strings.Contains(warning, "yaml") {
				t.Fatalf("warning must name exactly the missing deps: %q", warning)
			}
		})
	}
}
