package wsagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// variantBundle writes a hosted bundle as an installed package lays it out
// and returns its root.
func variantBundle(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "ws")
	outside := filepath.Join(t.TempDir(), "shared-skill.md")
	files := map[string]string{
		".claude-plugin/plugin.json": `{"name":"ws","version":"0.50.2","agents":["./claude-agents/a.md"],"hooks":"./hooks/hooks.json",
			"mcpServers":{"ws":{"command":"python3","args":["${CLAUDE_PLUGIN_ROOT}/bin/ws-mcp-launcher.py","serve","--stdio"],"env":{"WS_MCP_CLI_NAME":"ws-cli"}}}}`,
		".codex-plugin/plugin.json": `{}`,
		".mcp.json":                 `{"mcpServers":{"ws":{"command":"python3"}}}`,
		"bin/ws-mcp-launcher.py":    "# launcher\n",
		"claude-agents/a.md":        "agent\n",
		"hooks/hooks.json":          "{}\n",
		"hooks/mod.ts":              "export default 1\n",
		"skills/s/SKILL.md":         "skill\n",
		"config-manifest.json":      "{}\n",
		"tests/t.sh":                "test\n",
		"native-pi/package.json":    "{}\n",
	}
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(outside, []byte("linked skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(root, "skills", "s", "linked.md")); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestClaudeHTTPVariant: the variant is the bundle with its ws server
// replaced by the HTTP route under the same name; what Claude Code loads
// is copied (no path in it resolves outside it), the launcher's directory
// is linked, and entries no Claude session reads are left out. An equal
// bundle reuses the variant; a changed one gets its own.
func TestClaudeHTTPVariant(t *testing.T) {
	root, dir := variantBundle(t), filepath.Join(t.TempDir(), HostedVariantsDir)
	variant, err := claudeHTTPVariant(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(variant) != dir || !strings.HasPrefix(filepath.Base(variant), "ws-") {
		t.Fatalf("variant %s is the package's, under %s", variant, dir)
	}
	raw, err := os.ReadFile(filepath.Join(variant, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	servers, _ := manifest["mcpServers"].(map[string]any)
	ws, _ := servers["ws"].(map[string]any)
	headers, _ := ws["headers"].(map[string]any)
	if len(servers) != 1 || ws["type"] != "http" || ws["url"] != "${"+EnvMCPSessionURL+"}" || headers["Authorization"] != "Bearer ${"+EnvMCPSessionToken+"}" {
		t.Fatalf("the one server is ws over the HTTP route: %v", servers)
	}
	if manifest["name"] != "ws" || manifest["version"] != "0.50.2" || manifest["hooks"] != "./hooks/hooks.json" || !slices.Equal(anyStrings(manifest["agents"]), []string{"./claude-agents/a.md"}) {
		t.Fatalf("every other member is kept: %v", manifest)
	}
	for _, rel := range []string{"claude-agents/a.md", "hooks/mod.ts", "hooks/hooks.json", "skills/s/SKILL.md", "config-manifest.json"} {
		info, err := os.Lstat(filepath.Join(variant, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("%s is a copy: %v %v", rel, info, err)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Lstat(filepath.Join(variant, "skills", "s", "linked.md"))
		body, _ := os.ReadFile(filepath.Join(variant, "skills", "s", "linked.md"))
		if err != nil || !info.Mode().IsRegular() || string(body) != "linked skill\n" {
			t.Fatalf("a link in the bundle is copied as its target: %v %v %q", info, err, body)
		}
	}
	if target, err := os.Readlink(filepath.Join(variant, "bin")); err != nil || target != filepath.Join(root, "bin") {
		t.Fatalf("bin links to the bundle's: %q %v", target, err)
	}
	for _, rel := range []string{".mcp.json", ".codex-plugin", "tests", "native-pi"} {
		if _, err := os.Lstat(filepath.Join(variant, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s is left out: %v", rel, err)
		}
	}

	again, err := claudeHTTPVariant(root, dir)
	if err != nil || again != variant {
		t.Fatalf("an equal bundle reuses its variant: %s %v", again, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("one variant and no stage left: %v", entries)
	}
	mod := filepath.Join(root, "hooks", "mod.ts")
	if err := os.WriteFile(mod, []byte("export default 2 // changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(mod, later, later)
	changed, err := claudeHTTPVariant(root, dir)
	if err != nil || changed == variant {
		t.Fatalf("a changed bundle gets its own variant: %s %v", changed, err)
	}
	if body, _ := os.ReadFile(filepath.Join(changed, "hooks", "mod.ts")); string(body) != "export default 2 // changed\n" {
		t.Fatalf("the new variant holds the change: %q", body)
	}
	if body, _ := os.ReadFile(filepath.Join(variant, "hooks", "mod.ts")); string(body) != "export default 1\n" {
		t.Fatalf("the old variant, which a running lead may read, stays: %q", body)
	}
}

func TestClaudeHTTPVariantRefusesABadManifest(t *testing.T) {
	for name, manifest := range map[string]string{
		"no name":       `{"version":"1"}`,
		"path name":     `{"name":"../ws"}`,
		"not an object": `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".claude-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			if variant, err := claudeHTTPVariant(root, t.TempDir()); err == nil {
				t.Fatalf("built %s", variant)
			}
		})
	}
}

func anyStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}
