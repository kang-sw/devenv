package wsagent

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func piLeadFixture(t *testing.T) (StartSpec, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "agents-plugin-wsflow")
	restore := piBundledPluginRoot
	piBundledPluginRoot = func() (string, error) { return root, nil }
	t.Cleanup(func() { piBundledPluginRoot = restore })
	spec := StartSpec{Root: base, Cwd: filepath.Join(base, "project"), Dir: filepath.Join(base, "session"), Lead: &LeadSpec{}}
	if err := os.MkdirAll(piDir(spec.Dir), 0o700); err != nil {
		t.Fatal(err)
	}
	return spec, filepath.Join(base, "agents-plugin-pi")
}

func piLeadFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func piLeadBundle(t *testing.T, root string) string {
	t.Helper()
	piLeadFile(t, filepath.Join(root, "package.json"), `{"pi":{"extensions":["./lib/hosted.ts"]}}`)
	entry := filepath.Join(root, "lib", "hosted.ts")
	piLeadFile(t, entry, "export default function () {}")
	return entry
}

func TestPiLeadBundledEntries(t *testing.T) {
	_, root := piLeadFixture(t)
	entry := piLeadBundle(t, root)
	entries, err := piBundledEntries()
	if err != nil || !slices.Equal(entries, []string{entry}) {
		t.Fatalf("manifest entry = %v, %v", entries, err)
	}
	for _, text := range []string{`{`, `{}`, `{"pi":{"extensions":["../escape.ts"]}}`, `{"pi":{"extensions":[""]}}`, `{"pi":{"extensions":["gone.ts"]}}`} {
		piLeadFile(t, filepath.Join(root, "package.json"), text)
		if got, err := piBundledEntries(); err == nil {
			t.Fatalf("%s: accepted %v", text, got)
		}
	}
}

func TestPiLeadIsolatedLaunch(t *testing.T) {
	spec, root := piLeadFixture(t)
	entry := piLeadBundle(t, root)
	spec.Lead.PluginIsolation = "isolated"
	warning := "stale warning"
	spec.Lead.PluginWarning = func(text string) { warning = text }
	spec.Model, spec.Effort = "openai/test", "high"
	spec.SessionFile = filepath.Join(spec.Root, "terminal.jsonl")
	spec.Lead.WSMCP = func() (MCPCommand, error) { t.Fatal("bundled entry needs no fallback"); return MCPCommand{}, nil }
	command, args, err := piLeadLaunch("pi-test", []string{"cli-test"}, spec)
	if err != nil || command != "pi-test" || warning != "" {
		t.Fatalf("command %q, warning %q, error %v", command, warning, err)
	}
	if !slices.Equal(args, append([]string{"cli-test"}, append(piLeadArgs(spec), "--no-extensions", "--extension", entry)...)) {
		t.Fatalf("isolated args = %v", args)
	}
	for _, banned := range []string{"--no-skills", "--no-context-files", "--no-approve", "--no-tools"} {
		if slices.Contains(args, banned) {
			t.Fatalf("plugin isolation must not disable %s: %v", banned, args)
		}
	}
}

func TestPiLeadDefaultLaunchUsesHostSDK(t *testing.T) {
	for _, isolation := range []string{"", "default"} {
		t.Run(isolation, func(t *testing.T) {
			spec, root := piLeadFixture(t)
			entry := piLeadBundle(t, root)
			spec.Lead.PluginIsolation = isolation
			pkg := filepath.Join(spec.Root, "host-pi")
			cli, sdk := filepath.Join(pkg, "dist", "cli.js"), filepath.Join(pkg, "dist", "index.js")
			piLeadFile(t, filepath.Join(pkg, "package.json"), `{"name":"@earendil-works/pi-coding-agent"}`)
			piLeadFile(t, cli, "")
			piLeadFile(t, sdk, "")
			command, args, err := piLeadLaunch("host-node", []string{cli, "--offline"}, spec)
			if err != nil || command != "host-node" {
				t.Fatalf("command %q, error %v", command, err)
			}
			adapter := filepath.Join(piDir(spec.Dir), "ws-lead.mjs")
			realCLI, _ := filepath.EvalSymlinks(cli)
			realSDK := filepath.Join(filepath.Dir(realCLI), "index.js")
			want := append([]string{adapter, realSDK, realCLI, "--offline"}, append(piLeadArgs(spec), "--no-extensions", "--extension", entry)...)
			if !slices.Equal(args, want) {
				t.Fatalf("default args = %v, want %v", args, want)
			}
			if raw, err := os.ReadFile(adapter); err != nil || string(raw) != piLeadAdapter {
				t.Fatalf("adapter not written: %v", err)
			}
		})
	}
}

func TestPiLeadMissingBundleFallback(t *testing.T) {
	spec, _ := piLeadFixture(t)
	spec.Lead.PluginIsolation = "isolated"
	spec.Lead.Env = map[string]string{EnvHostedSessionKey: "lead-key"}
	calls, warning := 0, ""
	spec.Lead.PluginWarning = func(text string) { warning = text }
	spec.Lead.WSMCP = func() (MCPCommand, error) {
		calls++
		return MCPCommand{Command: "/bundled/ws", Args: []string{"mcp"}, Env: map[string]string{"WS_HOSTED_SESSION_KEY": "lead-key"}}, nil
	}
	_, args, err := piLeadLaunch("pi", nil, spec)
	bridge := filepath.Join(piDir(spec.Dir), piBridgeFile)
	if err != nil || calls != 1 || !strings.Contains(warning, "using the ws MCP bridge") || !slices.Equal(args, append(piLeadArgs(spec), "--no-extensions", "--extension", "builtin:mcp", "--extension", bridge)) {
		t.Fatalf("fallback args %v, calls %d, warning %q, error %v", args, calls, warning, err)
	}
	raw, err := os.ReadFile(bridge)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `const sessionKey = "lead-key";`) || !strings.Contains(text, `pi.on("before_agent_start"`) || slices.Contains(args, "--append-system-prompt") {
		t.Fatalf("fallback must supply its identity without replacing user prompt sources: %s", text)
	}
	var servers map[string]map[string]any
	if err := json.Unmarshal([]byte(text[strings.Index(text, "= ")+2:strings.Index(text, ";\n")]), &servers); err != nil {
		t.Fatal(err)
	}
	ws := servers[ReservedMCPServer]
	if len(servers) != 1 || ws["cwd"] != spec.Cwd || ws["command"] != "/bundled/ws" || ws["exposure"] != "direct" {
		t.Fatalf("fallback server = %v", servers)
	}
	if env := ws["env"].(map[string]any); env["WS_HOSTED_SESSION_KEY"] != "lead-key" {
		t.Fatalf("fallback lost lead identity: %v", env)
	}
	spec.Lead.WSMCP = nil
	if _, _, err := piLeadLaunch("pi", nil, spec); err == nil {
		t.Fatal("missing fallback must fail closed")
	}
	spec.Lead.WSMCP = func() (MCPCommand, error) { return MCPCommand{}, errors.New("factory failure") }
	if _, _, err := piLeadLaunch("pi", nil, spec); err == nil || !strings.Contains(err.Error(), "factory failure") {
		t.Fatalf("fallback failure = %v", err)
	}
}

func TestPiLeadUnixNPMSymlink(t *testing.T) {
	spec, root := piLeadFixture(t)
	piLeadBundle(t, root)
	pkg := filepath.Join(spec.Root, "node_modules", "@earendil-works", "pi-coding-agent")
	cli, sdk := filepath.Join(pkg, "dist", "cli.js"), filepath.Join(pkg, "dist", "index.js")
	piLeadFile(t, filepath.Join(pkg, "package.json"), `{"name":"@earendil-works/pi-coding-agent"}`)
	piLeadFile(t, cli, "")
	piLeadFile(t, sdk, "")
	link := filepath.Join(spec.Root, "pi")
	if err := os.Symlink(cli, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	restore := lookPath
	lookPath = func(name string) (string, error) {
		if name != "node" {
			t.Fatalf("unexpected lookup: %s", name)
		}
		return "host-node", nil
	}
	t.Cleanup(func() { lookPath = restore })
	command, args, err := piLeadLaunch(link, nil, spec)
	if err != nil || command != "host-node" {
		t.Fatalf("command %q, error %v", command, err)
	}
	// Parent temp directories may themselves be symlinks (e.g. macOS /var).
	wantCLI, _ := filepath.EvalSymlinks(cli)
	wantSDK := filepath.Join(filepath.Dir(wantCLI), "index.js")
	if len(args) < 3 || args[1] != wantSDK || args[2] != wantCLI {
		t.Fatalf("SDK must belong to the resolved CLI, not cwd: %v", args)
	}
}

func TestPiLeadStandaloneDefaultRefused(t *testing.T) {
	spec, root := piLeadFixture(t)
	piLeadBundle(t, root)
	command := filepath.Join(spec.Root, "standalone-pi")
	piLeadFile(t, command, "binary")
	if _, _, err := piLeadLaunch(command, nil, spec); err == nil || !strings.Contains(err.Error(), "JavaScript SDK") {
		t.Fatalf("cannot safely restore from a standalone binary without its SDK: %v", err)
	}
}

// Run the adapter against Pi itself when its SDK is available. The remote test
// host can pin WS_PI_TEST_SDK to <pi package>/dist/index.js; no model/API calls
// are made. The JS suite also always exercises the adapter with a fake resolver.
func TestPiLeadResolverAdapter(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	env := os.Environ()
	if os.Getenv("WS_PI_TEST_SDK") == "" {
		if command, pre, err := harnessCommand("", "pi"); err == nil {
			if _, sdk, err := piSDKPaths(command, pre); err == nil {
				env = append(env, "WS_PI_TEST_SDK="+sdk)
			}
		}
	}
	cmd := exec.Command(node, "--test", "testdata/pi-lead.test.mjs")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Pi resolver adapter: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
