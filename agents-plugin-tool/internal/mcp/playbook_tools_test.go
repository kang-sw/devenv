package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Test helpers
// ---------------------------------------------------------------------------

// buildTestRsrcTree creates a minimal rsrc tree for testing playbook tools.
// playbooks maps relative paths to file content.
// Returns the root path with a freshly generated manifest.json.
func buildTestRsrcTree(t *testing.T, playbooks map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relPath, content := range playbooks {
		full := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}
	m, err := wsrsrc.GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := wsrsrc.WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	return root
}

// newTestServerWithHarness creates a Server bound to a temp root with the given harness.
func newTestServerWithHarness(t *testing.T, harness string) *Server {
	t.Helper()
	s := NewServer(t.TempDir(), "test")
	if harness != "" {
		s.observeHarness("test", harness)
	}
	return s
}

func isolatedPlaybookConfigOptions(t *testing.T) wsconfig.Options {
	t.Helper()
	return wsconfig.Options{
		CacheHome:  filepath.Join(t.TempDir(), "cache"),
		ConfigHome: filepath.Join(t.TempDir(), "config"),
	}
}

func shippedImplementerContext() map[string]string {
	return map[string]string{
		"PlanPath":           "ai-docs/.plans/plan.md",
		"VerificationHint":   "go test ./internal/mcp -run TestRenderPlaybookShippedImplementerDeclaredContext",
		"ResultExpectations": "Report outcome, files changed, commits, verification, and blockers.",
		"CommitRangeHint":    "Report <first-commit>..<last-commit> after committing logical checkpoints.",
	}
}

// initGitRepo creates a git repository in a temp dir and returns its path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", c[1:], err, out)
		}
	}
	return dir
}

// asPlaybookError reports whether err or any error in its chain matches type T.
func asPlaybookError[T error](err error, target *T) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}

// writeTestFile writes content to root/relPath, creating parent dirs.
func writeTestFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// ---------------------------------------------------------------------------
// Fixture playbook content strings
// ---------------------------------------------------------------------------

const (
	// plainPlaybookContent: non-delegate, one custom variable.
	plainPlaybookContent = `---
kind: print
delegates: false
variables:
  - WorktreeID
---
# Plain Playbook

Worktree: {{.WorktreeID}}
`

	// delegatePlaybookContent: delegates:true with all terminology vars.
	// NOTE: kind:render is advisory metadata only — the loader does not restrict
	// by kind, so this fixture is valid for use with printPlaybook too. kind is
	// not a tool-routing gate; it is surfaced in PlaybookMeta for caller inspection.
	delegatePlaybookContent = `---
kind: render
delegates: true
variables:
  - ExploreAgent
  - SpawnIdiom
  - ContinueIdiom
---
# Delegate Playbook

Explore: {{.ExploreAgent}}
Spawn: {{.SpawnIdiom}}
Continue: {{.ContinueIdiom}}
`

	// modelAliasPlaybookContent: declares RoleModel, resolved from the playbook's tier.
	// tier: medium is used so the derivation path is exercised in tests.
	modelAliasPlaybookContent = `---
kind: print
delegates: false
tier: medium
variables:
  - RoleModel
---
# Model Alias Playbook

Model: {{.RoleModel}}
`

	// tierModelPlaybookContent: no declared variables — the four fixed-tier
	// model vars are unconditionally auto-injected (ImplicitVariableNames),
	// so they render without a frontmatter `variables:` declaration.
	tierModelPlaybookContent = `---
kind: print
delegates: false
---
# Tier Model Playbook

Small: {{.SmallTierModel}}
Medium: {{.MediumTierModel}}
Large: {{.LargeTierModel}}
XLarge: {{.XLargeTierModel}}
`

	// noVarsPlaybookContent: no variables, static content.
	noVarsPlaybookContent = `---
kind: print
delegates: false
---
# No-Vars Playbook

Static content only.
`
)

// ---------------------------------------------------------------------------
// MCP dispatch: tool surface (no wsflow gate, no no-agent gate)
// ---------------------------------------------------------------------------

func TestPlaybookToolsInLeadToolNames(t *testing.T) {
	names := LeadToolNames()
	has := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}
		return false
	}
	if !has("playbook.read") {
		t.Error("playbook.read missing from LeadToolNames")
	}
	if !has("playbook.render") {
		t.Error("playbook.render missing from LeadToolNames")
	}
}

func TestPlaybookToolsNotNoAgentHidden(t *testing.T) {
	if noAgentHiddenTool("playbook.read") {
		t.Error("playbook.read is incorrectly hidden in no-agent mode")
	}
	if noAgentHiddenTool("playbook.render") {
		t.Error("playbook.render is incorrectly hidden in no-agent mode")
	}
}

// TestConfigResolveAgentNotNoAgentHidden covers config.resolve_agent's
// no-agent applicability (260905 Phase 3): like config.list/config.tune, it
// is a generic tool with no configRegistry entry, so configKeyEntryForTool
// resolves not-found and noAgentHiddenTool's config.* branch falls through
// to "not hidden" — the tool stays visible in agentless/wsflow mode.
func TestConfigResolveAgentNotNoAgentHidden(t *testing.T) {
	if noAgentHiddenTool("config.resolve_agent") {
		t.Error("config.resolve_agent is incorrectly hidden in no-agent mode")
	}
}

func TestPlaybookToolsVisibleInToolsList(t *testing.T) {
	listed := map[string]bool{}
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		listed[name] = true
	}
	for _, want := range []string{"playbook.read", "playbook.render"} {
		if !listed[want] {
			t.Errorf("tool %q missing from tools() list", want)
		}
	}
}

func TestPlaybookToolsSchemaNameRequired(t *testing.T) {
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		if name != "playbook.read" && name != "playbook.render" {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		required, _ := schema["required"].([]string)
		found := false
		for _, r := range required {
			if r == "name" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool %q schema: 'name' not in required %v", name, required)
		}
	}
}

// ---------------------------------------------------------------------------
// MCP dispatch: end-to-end via callTool
// ---------------------------------------------------------------------------

func TestPlaybookPrintMCPDispatch(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"novars/novars.md": noVarsPlaybookContent,
	})
	useResourceFixture(t, rsrcRoot)

	srv := NewServer(t.TempDir(), "test")
	// req.Params is the JSON for the tools/call params object:
	// {"name": "<tool-name>", "arguments": {...}}
	reqParams, _ := json.Marshal(map[string]any{
		"name": "playbook.read",
		"arguments": map[string]any{
			"name": "novars",
		},
	})
	req := request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  reqParams,
	}
	resp := srv.callTool(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("callTool error: %v", resp.Error.Message)
	}
	result, _ := resp.Result.(map[string]any)
	if result["isError"] == true {
		if content, ok := result["content"].([]map[string]string); ok && len(content) > 0 {
			t.Fatalf("callTool isError: %s", content[0]["text"])
		}
		t.Fatal("callTool returned isError")
	}
	content, _ := result["content"].([]map[string]string)
	if len(content) == 0 {
		t.Fatal("callTool returned no content")
	}
	if !strings.Contains(content[0]["text"], "Static content only") {
		t.Errorf("callTool result %q: expected playbook content", content[0]["text"])
	}
}

// mustOverrideLookup builds the session-anchored prompt-override lookup and
// fails the test when any config scope fails to load.
func mustOverrideLookup(t *testing.T, s *Server, sessionKey string) overrideLookupFn {
	t.Helper()
	lookup, err := buildOverrideLookup(s, sessionKey)
	if err != nil {
		t.Fatalf("buildOverrideLookup: %v", err)
	}
	return lookup
}
