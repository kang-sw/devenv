package wsagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestParseProfileAcceptsFullProfile(t *testing.T) {
	raw := `{"description":"d","backend":"claude","model":"haiku","effort":"low",
		"tools":["Read","Bash(git status)"],"ws_tools":["tickets.query","project_tree"],
		"mcp_servers":{"docs":{"command":"docs-mcp"}},"plugins":["plug"],"skills":true,
		"instructions":"agent.md","idle_timeout_seconds":0,"list_in_index":true}`
	p, err := ParseProfile(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("ParseProfile: %v", err)
	}
	if p.IdleTimeout() != 0 {
		t.Fatalf("idle_timeout_seconds 0 should mean never, got %s", p.IdleTimeout())
	}
	if got := p.InstructionsPath("/root"); got != filepath.Join("/root", "agent.md") {
		t.Fatalf("relative instructions should resolve against the root, got %s", got)
	}
	if got := p.MCPServerNames(); len(got) != 1 || got[0] != "docs" {
		t.Fatalf("MCPServerNames = %v", got)
	}
	if ToolBase("Bash(git status)") != "Bash" {
		t.Fatalf("ToolBase should strip the permission rule")
	}
}

func TestParseProfileDefaults(t *testing.T) {
	p, err := ParseProfile(json.RawMessage(`{"backend":"pi"}`))
	if err != nil {
		t.Fatalf("ParseProfile: %v", err)
	}
	if p.IdleTimeout() != DefaultIdleTimeout || p.ListInIndex || p.Skills || len(p.Tools) != 0 || len(p.WSTools) != 0 || len(p.MCPServers) != 0 {
		t.Fatalf("an absent knob should default off: %+v", p)
	}
	if p.InstructionsPath("/root") != "" {
		t.Fatalf("no instructions should give no path")
	}
}

func TestParseProfileRefusesInvalid(t *testing.T) {
	cases := map[string]string{
		"not an object":    `["claude"]`,
		"unknown field":    `{"backend":"claude","tool":["Read"]}`,
		"missing backend":  `{"model":"haiku"}`,
		"unknown backend":  `{"backend":"codex"}`,
		"reserved server":  `{"backend":"claude","mcp_servers":{"ws":{"command":"x"}}}`,
		"server not obj":   `{"backend":"claude","mcp_servers":{"docs":"x"}}`,
		"negative idle":    `{"backend":"claude","idle_timeout_seconds":-1}`,
		"bad tool":         `{"backend":"claude","tools":["Read, Write"]}`,
		"unclosed rule":    `{"backend":"claude","tools":["Bash(git"]}`,
		"bad ws tool":      `{"backend":"claude","ws_tools":["Tickets Query"]}`,
		"trailing value":   `{"backend":"claude"} {}`,
		"blank instructns": `{"backend":"claude","instructions":"  "}`,
	}
	for name, raw := range cases {
		if _, err := ParseProfile(json.RawMessage(raw)); err == nil {
			t.Errorf("%s: ParseProfile(%s) should fail", name, raw)
		}
	}
	if err := ValidateProfileName("Bad_Name"); err == nil {
		t.Errorf("ValidateProfileName should refuse a name outside the pattern")
	}
}

// writeStructured writes a config file holding structured overrides.
func writeStructured(t *testing.T, path string, values map[string]string) {
	t.Helper()
	members := map[string]json.RawMessage{}
	for k, v := range values {
		members[k] = json.RawMessage(v)
	}
	raw, err := json.Marshal(map[string]any{"schema_version": 1, "structured_overrides": members})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// testConfig is an isolated config environment: empty project and global
// files unless a test writes them.
func testConfig(t *testing.T) wsconfig.Options {
	t.Helper()
	return wsconfig.Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}
}

func TestResolveProfileThroughWorkingRoot(t *testing.T) {
	opts := testConfig(t)
	withRepo, plain := t.TempDir(), t.TempDir()
	writeStructured(t, filepath.Join(opts.ConfigHome, "config.json"), map[string]string{
		ProfilePrefix + "chat":   `{"backend":"claude","model":"global-model"}`,
		ProfilePrefix + "broken": `{"backend":"nope"}`,
	})
	writeStructured(t, filepath.Join(withRepo, ".ws-workflow", "config.json"), map[string]string{
		ProfilePrefix + "chat": `{"backend":"claude","model":"repo-model"}`,
	})

	got, err := ResolveProfile(opts, withRepo, "chat")
	if err != nil {
		t.Fatalf("ResolveProfile(repo root): %v", err)
	}
	if got.Definition.Model != "repo-model" || got.Scope != wsconfig.ScopeRepo {
		t.Fatalf("the root's repo layer should win: %+v", got)
	}
	got, err = ResolveProfile(opts, plain, "chat")
	if err != nil {
		t.Fatalf("ResolveProfile(plain root): %v", err)
	}
	if got.Definition.Model != "global-model" || got.Scope != wsconfig.ScopeGlobal {
		t.Fatalf("a root without its own layer should fall back to global: %+v", got)
	}

	writeStructured(t, filepath.Join(opts.CacheHome, "config.json"), map[string]string{
		ProfilePrefix + "chat": `{"backend":"claude","model":"project-model"}`,
	})
	got, err = ResolveProfile(opts, withRepo, "chat")
	if err != nil || got.Definition.Model != "project-model" || got.Scope != wsconfig.ScopeProject {
		t.Fatalf("the project layer should win over repo: %+v, %v", got, err)
	}

	if _, err := ResolveProfile(opts, plain, "missing"); err == nil || !strings.Contains(err.Error(), "unknown agent profile") || !strings.Contains(err.Error(), "chat") {
		t.Fatalf("an unknown profile should fail listing the known ones, got %v", err)
	}
	if _, err := ResolveProfile(opts, plain, "broken"); err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("an invalid profile should fail naming the problem, got %v", err)
	}
	resolver := wsconfig.NewResolver(opts, nil, nil, nil)
	if _, err := ResolveProfiles(&resolver); err == nil {
		t.Fatalf("ResolveProfiles should fail on the broken entry")
	}
}

// TestRepoProfileMayNotStartPrograms: a committed profile that sets MCP
// servers or plugin directories never resolves, so no conversation launches
// from it; the same profile in a machine-local scope does.
func TestRepoProfileMayNotStartPrograms(t *testing.T) {
	opts := testConfig(t)
	root := t.TempDir()
	repoFile := filepath.Join(root, ".ws-workflow", "config.json")
	writeStructured(t, repoFile, map[string]string{
		ProfilePrefix + "servers": `{"backend":"claude","mcp_servers":{"docs":{"command":"docs-mcp"}}}`,
		ProfilePrefix + "plugins": `{"backend":"pi","plugins":["ext.ts"]}`,
		ProfilePrefix + "plain":   `{"backend":"claude","tools":["Read"],"ws_tools":["tickets.query"]}`,
	})
	for name, field := range map[string]string{"servers": "mcp_servers", "plugins": "plugins"} {
		_, err := ResolveProfile(opts, root, name)
		if err == nil || !strings.Contains(err.Error(), field) || !strings.Contains(err.Error(), repoFile) || !strings.Contains(err.Error(), "may not start programs") {
			t.Fatalf("repo profile %s should be refused naming %s and the file, got %v", name, field, err)
		}
	}
	if _, err := ResolveProfile(opts, root, "plain"); err != nil {
		t.Fatalf("a repo profile without executable fields resolves: %v", err)
	}

	writeStructured(t, filepath.Join(opts.ConfigHome, "config.json"), map[string]string{
		ProfilePrefix + "docs": `{"backend":"claude","mcp_servers":{"docs":{"command":"docs-mcp"}},"plugins":["plug"]}`,
	})
	if got, err := ResolveProfile(opts, t.TempDir(), "docs"); err != nil || got.Scope != wsconfig.ScopeGlobal {
		t.Fatalf("a global profile may set mcp_servers and plugins: %+v, %v", got, err)
	}
	if err := CheckProfileScope(Profile{Backend: BackendClaude, Plugins: []string{"p"}}, wsconfig.ScopeProject); err != nil {
		t.Fatalf("project scope may set plugins: %v", err)
	}
}

func TestIdleTimeoutUnits(t *testing.T) {
	n := 90
	if (Profile{IdleTimeoutSeconds: &n}).IdleTimeout() != 90*time.Second {
		t.Fatalf("idle_timeout_seconds should be seconds")
	}
}
