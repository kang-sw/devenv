package wsagent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func TestPiPreparedSourceDriftWithoutRebuiltBundleUsesBridge(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(base, "config"))
	owner := filepath.Join(base, "agents-plugin-wsflow")
	copyPiOwner(t, filepath.Join("..", "..", "..", "agents-plugin-wsflow"), owner)
	sibling := copyPiSourceInputs(t, owner, base)
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(owner, "rsrc"))
	entries, err := piBundledEntries()
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0], filepath.Join(base, "cache")) {
		t.Fatalf("current source payload must stay local: %v, %v", entries, err)
	}
	path := filepath.Join(sibling, filepath.FromSlash(piBundleSource))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	piLeadFile(t, path, string(raw)+"\n// development edit\n")
	entries, err = piBundledEntries()
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale sibling bundle must not select unchecked source: %v, %v", entries, err)
	}
	warning := "previous warning"
	spec := StartSpec{Root: base, Dir: filepath.Join(base, "session"), Lead: &LeadSpec{PluginIsolation: "isolated", Env: map[string]string{EnvHostedSessionKey: "fixture-key"}}}
	spec.Lead.PluginWarning = func(text string) { warning = text }
	spec.Lead.WSMCP = func() (MCPCommand, error) { return MCPCommand{Command: "fixture-only"}, nil }
	_, args, err := piLeadLaunch("pi", nil, spec)
	if err != nil || !strings.Contains(warning, "EXTENSION NOT LOADED") || !strings.Contains(warning, "stale") || !slices.Contains(args, "builtin:mcp") || slices.Contains(args, path) {
		t.Fatalf("stale source launch = %v, warning %q, error %v", args, warning, err)
	}
}

func TestPiPreparedPackageOnlyStaleBundleUsesBridge(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 unavailable")
	}
	base := t.TempDir()
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(base, "config"))
	owner := filepath.Join(base, "plugin")
	copyPiOwner(t, filepath.Join("..", "..", "..", "agents-plugin"), owner)
	// Keep the plain-file checksum inventory consistent while making the bundle
	// header stale. This is prepared-file invalidity, not checkout source drift.
	script := `import hashlib, json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "native-pi"
m = json.loads((p / "first-party-manifest.json").read_text())
bundle = p / "first-party/dist/index.js"
lines = bundle.read_bytes().split(b"\n", 1)
bundle.write_bytes(b"// ws-pi-bundle-inputs: sha256-" + b"0" * 64 + b"\n" + lines[1])
m["files"]["dist/index.js"] = hashlib.sha256(bundle.read_bytes()).hexdigest()
(p / "first-party-manifest.json").write_text(json.dumps(m))
`
	if out, err := exec.Command(python, "-I", "-c", script, owner).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(owner, "rsrc"))
	warning := ""
	spec := StartSpec{Root: base, Dir: filepath.Join(base, "session"), Lead: &LeadSpec{PluginIsolation: "isolated", Env: map[string]string{EnvHostedSessionKey: "fixture-key"}}}
	spec.Lead.PluginWarning = func(text string) { warning = text }
	spec.Lead.WSMCP = func() (MCPCommand, error) { return MCPCommand{Command: "fixture-only"}, nil }
	_, args, err := piLeadLaunch("pi", nil, spec)
	if err != nil || !strings.Contains(warning, "bundle is stale") || !strings.Contains(warning, "using the ws MCP bridge") || !slices.Contains(args, "builtin:mcp") {
		t.Fatalf("stale package-only launch = %v, warning %q, error %v", args, warning, err)
	}
}

func TestPiPreparedPayloadSeparatesDiagnostics(t *testing.T) {
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
	root := filepath.Join(base, "payload")
	piLeadFile(t, filepath.Join(root, "package.json"), "{}")
	helper := filepath.Join(base, "bin", "ws-pi-payload.py")
	piLeadFile(t, helper, fmt.Sprintf("import sys\nprint('selection reason', file=sys.stderr)\nprint(%q)\n", root))
	if got, err := piPreparedPayload(base); err != nil || got != root {
		t.Fatalf("stderr polluted payload path: %q, %v", got, err)
	}
	piLeadFile(t, helper, "import sys\nprint('preparation reason', file=sys.stderr)\nsys.exit(4)\n")
	if _, err := piPreparedPayload(base); err == nil || !strings.Contains(err.Error(), "preparation reason") {
		t.Fatalf("stderr missing from failure: %v", err)
	}
}
